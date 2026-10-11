//go:build windows

package installer

func setRetirementPathPrivacy(path string, _ bool) error {
	return Apply(path)
}

func retirementPathPrivacySatisfied(path string, _ bool) (bool, error) {
	return OwnerOnly(path)
}
