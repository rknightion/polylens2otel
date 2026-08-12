package phonetarget

import (
	"context"
	"errors"

	"github.com/rknightion/polylens2otel/internal/lensclient"
)

const localAdminPasswordAttribute = "device.auth.localAdminPassword" //nolint:gosec // Lens configuration attribute name, not a credential.

const localAdminPasswordQuery = `query PhoneLocalAdminPassword($policyID: String!) {
  getPolicyById(policyId: $policyID) {
    configurationAttributes { name value }
  }
}`

// LensQuery is the read-only query seam implemented by lensclient.Client.
type LensQuery interface {
	Query(context.Context, string, any, any) error
}

// PolicyIDSource selects the winning Lens policy for a device. Selection is a
// separate seam because deviceSearch does not expose group-policy precedence.
type PolicyIDSource interface {
	WinningPolicyID(context.Context, lensclient.Device) (string, error)
}

type PolicyIDFunc func(context.Context, lensclient.Device) (string, error)

func (f PolicyIDFunc) WinningPolicyID(ctx context.Context, device lensclient.Device) (string, error) {
	return f(ctx, device)
}

// LensPolicySource reads the local admin password from an already-selected
// winning policy. It never retains or returns upstream error text because an
// HTTP error body for this query could contain the policy credential.
type LensPolicySource struct {
	query     LensQuery
	policyIDs PolicyIDSource
}

func NewLensPolicySource(query LensQuery, policyIDs PolicyIDSource) (*LensPolicySource, error) {
	if query == nil || policyIDs == nil {
		return nil, errors.New("lens query and winning-policy source are required")
	}
	return &LensPolicySource{query: query, policyIDs: policyIDs}, nil
}

func (s *LensPolicySource) LocalAdminPassword(ctx context.Context, device lensclient.Device) (Secret, error) {
	policyID, err := s.policyIDs.WinningPolicyID(ctx, device)
	if err != nil || policyID == "" {
		return Secret{}, errors.New("resolve winning Lens policy")
	}
	var response struct {
		Policy struct {
			Attributes []struct {
				Name  string `json:"name"`
				Value Secret `json:"value"`
			} `json:"configurationAttributes"`
		} `json:"getPolicyById"`
	}
	if err := s.query.Query(ctx, localAdminPasswordQuery, map[string]string{"policyID": policyID}, &response); err != nil {
		return Secret{}, errors.New("read Lens policy configuration attributes")
	}
	for _, attribute := range response.Policy.Attributes {
		if attribute.Name == localAdminPasswordAttribute && !attribute.Value.empty() {
			return attribute.Value, nil
		}
	}
	return Secret{}, errors.New("lens policy has no local admin password")
}
