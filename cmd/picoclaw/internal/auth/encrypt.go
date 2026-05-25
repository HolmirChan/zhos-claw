package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/pkg/credential"
)

func newEncryptCommand() *cobra.Command {
	var sshKeyPath string
	var passphrase string

	cmd := &cobra.Command{
		Use:   "encrypt <api-key>",
		Short: "Encrypt an API key into enc:// format for model_list",
		Long: `Encrypt a plaintext API key with a passphrase and an SSH private key.

The encrypted output can be used directly as an api_keys entry in model_list.

SSH key lookup order:
  1. --ssh-key flag
  2. ~/.ssh/zhosclaw_ed25519.key

Examples:
  picoclaw auth encrypt -p "mypass" "sk-abc123"
  picoclaw auth encrypt -p "mypass" --ssh-key ~/.ssh/my.key "sk-abc123"`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if passphrase == "" {
				return fmt.Errorf("passphrase is required, use -p <passphrase>")
			}

			enc, err := credential.Encrypt(passphrase, sshKeyPath, args[0])
			if err != nil {
				return fmt.Errorf("encrypt failed: %w", err)
			}
			fmt.Println(enc)
			return nil
		},
	}

	cmd.Flags().StringVarP(&passphrase, "passphrase", "p", "",
		"Encryption passphrase")
	cmd.Flags().StringVar(&sshKeyPath, "ssh-key", "",
		"Path to SSH private key (default: ~/.ssh/zhosclaw_ed25519.key)")

	return cmd
}
