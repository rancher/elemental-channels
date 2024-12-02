/*
Copyright © 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package updatesparser

import (
	"fmt"
	"os"

	"github.com/davidcassany/updateinfo-parser/pkg/parser"
	"github.com/spf13/cobra"
)

const securityType = "security"

var rootCmd = &cobra.Command{
	Use:   "updatesparser [flags] updateinfo",
	Short: "updatesparser - A simple CLI to parser updateinfo XML files",
	Long:  `A simple CLI to parser updateinfo XML files`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		updateInfo := args[0]
		if _, err := os.Stat(updateInfo); err != nil {
			return fmt.Errorf("could not fild updateinfo file '%s'", updateInfo)
		}
		flags := cmd.Flags()

		beforeStr, _ := flags.GetString("beforeDate")
		afterStr, _ := flags.GetString("afterDate")
		packagesF, _ := flags.GetString("packages")
		tmplF, _ := flags.GetString("template")
		output, _ := flags.GetString("output")
		sec, _ := flags.GetBool("security")
		json, _ := flags.GetBool("json")

		fOpts := []parser.FilterOpt{}
		if beforeStr != "" {
			fOpts = append(fOpts, parser.WithBeforeTime(beforeStr))
		}
		if afterStr != "" {
			fOpts = append(fOpts, parser.WithBeforeTime(afterStr))
		}
		if packagesF != "" {
			fOpts = append(fOpts, parser.WithPackagesFile(packagesF))
		}
		if sec {
			fOpts = append(fOpts, parser.WithUpdateType(securityType))
		}
		fCfg, err := parser.NewFilterConfig(fOpts...)
		if err != nil {
			return err
		}

		oOpts := []parser.OutputOpt{}
		if output != "" {
			oOpts = append(oOpts, parser.WithOutputFile(output))
		}
		if tmplF != "" {
			oOpts = append(oOpts, parser.WithTemplateFile(tmplF))
		}
		if json {
			oOpts = append(oOpts, parser.WithJsonOutput())
		}

		oCfg, err := parser.NewOutputConfig(oOpts...)
		if err != nil {
			return err
		}

		return parser.ParseFileToOutput(updateInfo, *fCfg, *oCfg)
	},
}

func init() {
	rootCmd.Flags().StringP("beforeDate", "b", "", "Filter updates released before the given date. Date as a unix timestamp")
	rootCmd.Flags().StringP("afterDate", "a", "", "Filter updates released after the given date. Date as a unix timestamp")
	rootCmd.Flags().StringP("output", "o", "", "Output file. Defaults to 'stdout'")
	rootCmd.Flags().StringP("template", "t", "", "Provides a custom update template file")
	rootCmd.Flags().StringP("packages", "p", "", "Package file list to filter updates modiying any of listed packages")
	rootCmd.Flags().BoolP("security", "s", false, "Match only security updates")
	rootCmd.Flags().BoolP("json", "j", false, "Output in json format")
	rootCmd.MarkFlagsMutuallyExclusive("json", "template")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error: %v\n", err)
		os.Exit(1)
	}
}
