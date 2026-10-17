package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// credentialFlags are a credential a command brings, on top of those gitd keeps. A token
// is read from an environment variable, never given on the command line.
type credentialFlags struct {
	tokenEnv, tokenUser, sshKey, prefix string
}

func addCredentialFlags(command *cobra.Command) *credentialFlags {
	flags := &credentialFlags{}
	command.Flags().StringVar(&flags.tokenEnv, "token-env", "", "read a token from this environment variable")
	command.Flags().StringVar(&flags.tokenUser, "token-user", "", "the user name the token goes with (default: the host's)")
	command.Flags().StringVar(&flags.sshKey, "ssh-key", "", "read with the SSH key at this path")
	command.Flags().StringVar(&flags.prefix, "prefix", "", "the repositories the credential opens (default: any)")
	command.MarkFlagsMutuallyExclusive("token-env", "ssh-key")
	return flags
}

// proto is the credential as a request carries it, or none.
func (flags *credentialFlags) proto() ([]*gitdv1.Credential, error) {
	switch {
	case flags.tokenEnv != "":
		token := strings.TrimSpace(os.Getenv(flags.tokenEnv))
		if token == "" {
			return nil, fmt.Errorf("$%s is empty", flags.tokenEnv)
		}
		return []*gitdv1.Credential{{
			UrlPrefix: flags.prefix, HttpsUser: flags.tokenUser,
			Secret: &gitdv1.Credential_HttpsToken{HttpsToken: token},
		}}, nil
	case flags.sshKey != "":
		if flags.tokenUser != "" {
			return nil, errors.New("--token-user goes with --token-env")
		}
		return []*gitdv1.Credential{{
			UrlPrefix: flags.prefix, Secret: &gitdv1.Credential_SshKeyPath{SshKeyPath: flags.sshKey},
		}}, nil
	}
	return nil, nil
}
