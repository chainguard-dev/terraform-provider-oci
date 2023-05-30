package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/structure"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"
)

func main() {
	var files, envs []string
	var platform string

	cmd := &cobra.Command{
		Use:          "check",
		Short:        "Check a container image for compliance with a set of conditions",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := name.ParseReference(args[0])
			if err != nil {
				return fmt.Errorf("failed to parse reference: %v", err)
			}
			plat, err := v1.ParsePlatform(platform)
			if err != nil {
				return fmt.Errorf("failed to parse platform: %v", err)
			}
			img, err := remote.Image(ref,
				remote.WithAuthFromKeychain(authn.DefaultKeychain),
				remote.WithPlatform(*plat),
			)
			if err != nil {
				return fmt.Errorf("failed to fetch image: %v", err)
			}

			var conds structure.Conditions
			fc := structure.FilesCondition{Want: map[string]structure.File{}}
			for _, f := range files {
				path, regex, _ := strings.Cut(f, "=")
				fc.Want[path] = structure.File{Regexp: regexp.MustCompile(regex).String()}
			}
			conds = append(conds, fc)

			ec := structure.EnvCondition{Want: map[string]string{}}
			for _, e := range envs {
				k, v, _ := strings.Cut(e, "=")
				ec.Want[k] = v
			}
			conds = append(conds, ec)

			return conds.Check(img)
		},
	}
	cmd.Flags().StringSliceVarP(&files, "file", "f", nil, `Files to check (e.g., "/etc/passwd=.*nonroot:.*" or "/etc/passwd" to check existence only)`)
	cmd.Flags().StringSliceVarP(&envs, "env", "e", nil, `Environment variables to check (e.g., "PATH=/usr/local/bin")`)
	cmd.Flags().StringVar(&platform, "platform", "linux/amd64", "Platform to check (e.g., linux/amd64)")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
