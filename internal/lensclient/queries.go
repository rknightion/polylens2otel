package lensclient

import "context"

// Tenant is a Poly Lens tenant.
type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Device is the inventory shape needed by Lens and phone collectors.
type Device struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Connected             bool   `json:"connected"`
	LastDetected          string `json:"lastDetected"`
	LastConfigRequestDate string `json:"lastConfigRequestDate"`
	SoftwareVersion       string `json:"softwareVersion"`
	SoftwareBuild         string `json:"softwareBuild"`
	InternalIP            string `json:"internalIp"`
	ExternalIP            string `json:"externalIp"`
	HardwareModel         string `json:"hardwareModel"`
	ProductID             string `json:"productId"`
	MACAddress            string `json:"macAddress"`
	HardwareRevision      string `json:"hardwareRevision"`
	Site                  *Site  `json:"site"`
	Room                  *Site  `json:"room"`
}

type Site struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Call struct {
	ID    string `json:"id"`
	State string `json:"state"`
}
type Firmware struct {
	Version        string  `json:"version"`
	ReleaseChannel *string `json:"releaseChannel"`
}

const tenantsQuery = `{ tenants { id name } }`

const devicesQuery = `query Devices($tenantID: ID!, $params: DeviceFindArgs!) {
  tenant(id: $tenantID) { inventory { deviceSearch(params: $params) {
    pageInfo { totalCount hasNextPage nextToken }
    edges { node { id name connected lastDetected lastConfigRequestDate softwareVersion softwareBuild internalIp externalIp hardwareModel productId macAddress hardwareRevision site { id name } room { id name } } }
  } } }
}`

const activeCallsQuery = `query ActiveCalls($deviceID: ID!) { activeCalls(deviceId: $deviceID) { calls { id state } } }`
const firmwareQuery = `query Firmware($pid: String!) { availableProductSoftwareByPid(pid: $pid) { version releaseChannel } }`

func (c *Client) Tenants(ctx context.Context) ([]Tenant, error) {
	var response struct {
		Tenants []Tenant `json:"tenants"`
	}
	if err := c.Query(ctx, tenantsQuery, nil, &response); err != nil {
		return nil, err
	}
	return response.Tenants, nil
}

func (c *Client) Devices(ctx context.Context, tenantID string) ([]Device, error) {
	var all []Device
	var nextToken *string
	for {
		params := map[string]any{"pageSize": c.cfg.PageSize}
		if nextToken != nil {
			params["nextToken"] = *nextToken
		}
		var response struct {
			Tenant struct {
				Inventory struct {
					DeviceSearch struct {
						PageInfo struct {
							HasNextPage bool    `json:"hasNextPage"`
							NextToken   *string `json:"nextToken"`
						} `json:"pageInfo"`
						Edges []struct {
							Node Device `json:"node"`
						} `json:"edges"`
					} `json:"deviceSearch"`
				} `json:"inventory"`
			} `json:"tenant"`
		}
		if err := c.Query(ctx, devicesQuery, map[string]any{"tenantID": tenantID, "params": params}, &response); err != nil {
			return nil, err
		}
		for _, edge := range response.Tenant.Inventory.DeviceSearch.Edges {
			all = append(all, edge.Node)
		}
		page := response.Tenant.Inventory.DeviceSearch.PageInfo
		if !page.HasNextPage {
			return all, nil
		}
		if page.NextToken == nil || *page.NextToken == "" {
			return nil, ErrMissingNextToken
		}
		nextToken = page.NextToken
	}
}

var ErrMissingNextToken = errMissingNextToken{}

type errMissingNextToken struct{}

func (errMissingNextToken) Error() string {
	return "Lens device pagination reported another page without nextToken"
}

func (c *Client) ActiveCalls(ctx context.Context, deviceID string) ([]Call, error) {
	var response struct {
		ActiveCalls struct {
			Calls []Call `json:"calls"`
		} `json:"activeCalls"`
	}
	if err := c.Query(ctx, activeCallsQuery, map[string]string{"deviceID": deviceID}, &response); err != nil {
		return nil, err
	}
	return response.ActiveCalls.Calls, nil
}

func (c *Client) LatestFirmware(ctx context.Context, productID string) (Firmware, error) {
	var response struct {
		Firmware Firmware `json:"availableProductSoftwareByPid"`
	}
	if err := c.Query(ctx, firmwareQuery, map[string]string{"pid": productID}, &response); err != nil {
		return Firmware{}, err
	}
	return response.Firmware, nil
}
