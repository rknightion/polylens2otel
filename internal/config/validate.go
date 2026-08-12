package config

import "fmt"

const maxPhoneConfigParams = 50

func validatePhoneConfigParams(params []string) error {
	if len(params) > maxPhoneConfigParams {
		return fmt.Errorf("phone.config_params must contain at most %d parameters", maxPhoneConfigParams)
	}
	return nil
}
