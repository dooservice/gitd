package cli

import "testing"

func TestACommandsCredentialComesFromTheEnvironmentOrAKeyPath(t *testing.T) {
	t.Setenv("GITD_CLI_TOKEN", " secret \n")
	given, err := (&credentialFlags{tokenEnv: "GITD_CLI_TOKEN", prefix: "https://github.com/acme"}).proto()
	if err != nil || len(given) != 1 || given[0].GetHttpsToken() != "secret" || given[0].GetUrlPrefix() != "https://github.com/acme" {
		t.Fatalf("token = %v, %v", given, err)
	}
	given, err = (&credentialFlags{sshKey: "/keys/web"}).proto()
	if err != nil || given[0].GetSshKeyPath() != "/keys/web" {
		t.Fatalf("key = %v, %v", given, err)
	}
	if given, err := (&credentialFlags{}).proto(); err != nil || given != nil {
		t.Errorf("none = %v, %v", given, err)
	}
	t.Setenv("GITD_CLI_EMPTY", "")
	if _, err := (&credentialFlags{tokenEnv: "GITD_CLI_EMPTY"}).proto(); err == nil {
		t.Error("an empty token was accepted")
	}
}
