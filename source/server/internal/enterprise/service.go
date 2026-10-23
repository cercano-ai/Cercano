package enterprise

// EnterpriseServiceName is the keychain service holding a profile's enterprise
// credentials. The default profile keeps the historical "cercano-enterprise"
// service so existing sign-ins stay valid.
func EnterpriseServiceName(profile string) string {
	if profile == "default" {
		return "cercano-enterprise"
	}
	return "cercano-enterprise." + profile
}
