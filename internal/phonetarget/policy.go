package phonetarget

import (
	"context"
	"errors"

	"github.com/rknightion/polylens2otel/internal/lensclient"
)

const localAdminPasswordAttribute = "device.auth.localAdminPassword" //nolint:gosec // Lens configuration attribute name, not a credential.

const winningPolicySourceQuery = `query WinningPolicySource($tenantID: String!, $deviceID: String!) {
  getDeviceParametersExtended(tenantId: $tenantID, deviceId: $deviceID, scope: DEVICE, sendOnlyChanged: false) {
    name
    policyDeploymentScope
    collectionId
    collectionName
  }
}`

const winningPoliciesQuery = `query WinningPolicies(
  $tenantID: String!
  $deviceID: String
  $deviceModel: String
  $siteID: String
  $groupID: String
  $policyFamilyID: String
  $scope: PolicyDeploymentScope!
) {
  getPolicies(
    tenantId: $tenantID
    deviceId: $deviceID
    deviceModel: $deviceModel
    siteId: $siteID
    groupId: $groupID
    policyFamilyId: $policyFamilyID
    policyScope: $scope
  ) {
    policyId
    configurationAttributes { name }
  }
}`

const devicePolicyFamilyQuery = `query DevicePolicyFamily($deviceID: String!) {
  device(id: $deviceID) {
    model { hardwareFamily { policyFamilyId } }
  }
}`

const localAdminPasswordQuery = `query PhoneLocalAdminPassword($policyID: String!) {
  getPolicyById(policyId: $policyID) {
    configurationAttributes { name currentValue }
  }
}`

// LensQuery is the read-only query seam implemented by lensclient.Client.
type LensQuery interface {
	Query(context.Context, string, any, any) error
}

// LensPolicySource asks Lens's parameter resolver which source won, maps that
// source to one policy, and reads the credential from that policy. It never
// retains or returns upstream error text because a response could contain the
// policy credential.
type LensPolicySource struct {
	query LensQuery
}

func NewLensPolicySource(query LensQuery) (*LensPolicySource, error) {
	if query == nil {
		return nil, errors.New("lens query is required")
	}
	return &LensPolicySource{query: query}, nil
}

func (s *LensPolicySource) LocalAdminPassword(ctx context.Context, device lensclient.Device) (Secret, error) {
	policyID, err := s.winningPolicyID(ctx, device)
	if err != nil {
		return Secret{}, err
	}
	var response struct {
		Policy struct {
			Attributes []struct {
				Name         string `json:"name"`
				CurrentValue Secret `json:"currentValue"`
			} `json:"configurationAttributes"`
		} `json:"getPolicyById"`
	}
	if err := s.query.Query(ctx, localAdminPasswordQuery, map[string]string{"policyID": policyID}, &response); err != nil {
		return Secret{}, errors.New("read Lens policy configuration attributes")
	}
	for _, attribute := range response.Policy.Attributes {
		if attribute.Name == localAdminPasswordAttribute && !attribute.CurrentValue.empty() {
			return attribute.CurrentValue, nil
		}
	}
	return Secret{}, errors.New("lens policy has no local admin password")
}

type winningPolicySource struct {
	Name         string `json:"name"`
	Scope        string `json:"policyDeploymentScope"`
	CollectionID string `json:"collectionId"`
}

func (s *LensPolicySource) winningPolicyID(ctx context.Context, device lensclient.Device) (string, error) {
	if device.TenantID == "" || device.ID == "" {
		return "", errors.New("resolve winning Lens policy source")
	}
	var sourceResponse struct {
		Parameters []winningPolicySource `json:"getDeviceParametersExtended"`
	}
	if err := s.query.Query(ctx, winningPolicySourceQuery, map[string]string{
		"tenantID": device.TenantID,
		"deviceID": device.ID,
	}, &sourceResponse); err != nil {
		return "", errors.New("resolve winning Lens policy source")
	}
	sources := make([]winningPolicySource, 0, 1)
	for _, parameter := range sourceResponse.Parameters {
		if parameter.Name == localAdminPasswordAttribute {
			sources = append(sources, parameter)
		}
	}
	if len(sources) != 1 {
		return "", errors.New("resolve one winning Lens policy source")
	}

	variables, err := s.policyVariables(ctx, device, sources[0])
	if err != nil {
		return "", err
	}
	var policyResponse struct {
		Policies []struct {
			ID         string `json:"policyId"`
			Attributes []struct {
				Name string `json:"name"`
			} `json:"configurationAttributes"`
		} `json:"getPolicies"`
	}
	if err := s.query.Query(ctx, winningPoliciesQuery, variables, &policyResponse); err != nil {
		return "", errors.New("resolve winning Lens policy")
	}
	policyIDs := make([]string, 0, 1)
	for _, policy := range policyResponse.Policies {
		for _, attribute := range policy.Attributes {
			if attribute.Name == localAdminPasswordAttribute && policy.ID != "" {
				policyIDs = append(policyIDs, policy.ID)
				break
			}
		}
	}
	if len(policyIDs) != 1 {
		return "", errors.New("resolve one winning Lens policy")
	}
	return policyIDs[0], nil
}

func (s *LensPolicySource) policyVariables(ctx context.Context, device lensclient.Device, source winningPolicySource) (map[string]any, error) {
	variables := map[string]any{
		"tenantID": device.TenantID,
		"scope":    source.Scope,
	}
	switch source.Scope {
	case "DEVICE":
		variables["deviceID"] = device.ID
	case "DEVICE_MODEL":
		if device.HardwareModel == "" {
			return nil, errors.New("resolve Lens device-model policy")
		}
		variables["deviceModel"] = device.HardwareModel
	case "SITE", "FAMILY_SITE":
		if source.CollectionID == "" {
			return nil, errors.New("resolve Lens site policy")
		}
		variables["siteID"] = source.CollectionID
		if source.Scope == "SITE" {
			if device.HardwareModel == "" {
				return nil, errors.New("resolve Lens site policy")
			}
			variables["deviceModel"] = device.HardwareModel
		}
	case "GROUP", "FAMILY_GROUP":
		if source.CollectionID == "" {
			return nil, errors.New("resolve Lens group policy")
		}
		variables["groupID"] = source.CollectionID
		if source.Scope == "GROUP" {
			if device.HardwareModel == "" {
				return nil, errors.New("resolve Lens group policy")
			}
			variables["deviceModel"] = device.HardwareModel
		}
	case "FAMILY_MODEL":
	case "GLOBAL":
		// Tenant ID is the complete selector for a global policy.
	default:
		return nil, errors.New("unsupported Lens policy scope")
	}
	if source.Scope == "FAMILY_MODEL" || source.Scope == "FAMILY_SITE" || source.Scope == "FAMILY_GROUP" {
		familyID, err := s.devicePolicyFamily(ctx, device.ID)
		if err != nil {
			return nil, err
		}
		variables["policyFamilyID"] = familyID
	}
	return variables, nil
}

func (s *LensPolicySource) devicePolicyFamily(ctx context.Context, deviceID string) (string, error) {
	var response struct {
		Device struct {
			Model struct {
				Family struct {
					ID string `json:"policyFamilyId"`
				} `json:"hardwareFamily"`
			} `json:"model"`
		} `json:"device"`
	}
	if err := s.query.Query(ctx, devicePolicyFamilyQuery, map[string]string{"deviceID": deviceID}, &response); err != nil || response.Device.Model.Family.ID == "" {
		return "", errors.New("resolve Lens device policy family")
	}
	return response.Device.Model.Family.ID, nil
}
