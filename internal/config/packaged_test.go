package config

import "testing"

func TestThePackagedConfigurationIsTheDefaults(t *testing.T) {
	packaged, err := Load("../../packaging/gitd.toml")
	if err != nil {
		t.Fatal(err)
	}
	defaults := Default()
	if packaged.Socket != defaults.Socket || packaged.State != defaults.State || packaged.GitHub != defaults.GitHub ||
		packaged.Audit != defaults.Audit || packaged.Limits != defaults.Limits || len(packaged.Publish.Settings) != 0 {
		t.Errorf("packaging/gitd.toml says other than the defaults: %+v", packaged)
	}
}
