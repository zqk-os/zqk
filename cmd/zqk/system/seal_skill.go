package system

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/skill"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewSealSkillCmd wires seal-skill from the command-spec builder.
func NewSealSkillCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSealSkillCommandBuilder()
	cmd.RunE = runSealSkill
	return cmd
}

func runSealSkill(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		if len(args) != 1 {
			return errfmt.Errorf("skill name required")
		}
		verifyOnly, err := cmd.Flags().GetBool("verify")
		if err != nil {
			return err
		}
		version, err := cmd.Flags().GetString("version")
		if err != nil {
			return err
		}
		issuer, err := cmd.Flags().GetString("issuer")
		if err != nil {
			return err
		}

		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			return errfmt.Errorf("project root not found")
		}

		skillName := args[0]
		skillDir := filepath.Join(projectRoot, ".zqk", "skills", skillName)
		skillPath := filepath.Join(skillDir, "SKILL.md")

		contentBytes, err := fileutil.ReadFile(skillPath)
		if err != nil {
			return errfmt.Newf("read skill file %s", skillPath).Wrap(err)
		}
		content := string(contentBytes)

		logger := logging.GetLoggerFromProfile("system")
		if verifyOnly {
			res := skill.VerifySeal(content)
			payload := map[string]any{
				"skill":                 skillName,
				objects.FieldKeyPath:    skillPath,
				"valid":                 res.Valid,
				objects.FieldKeyVersion: res.Version,
				"issuer":                res.Issuer,
				objects.FieldKeyStatus:  "ok",
			}
			if !res.Date.IsZero() {
				payload[objects.FieldKeyDate] = res.Date.Format(time.RFC3339)
			}
			if !res.Valid {
				payload[objects.FieldKeyStatus] = "failed"
				payload["error"] = res.Error
				_ = cli.FormatOutput(cmd, payload)
				return errfmt.Errorf("seal verification failed: %s", res.Error)
			}
			logger.Info("Skill seal verified", logging.String("skill", skillName), logging.String("path", skillPath))
			return cli.FormatOutput(cmd, payload)
		}

		parts := strings.SplitN(content, "---", 3)
		var bodyContent string
		var newLines []string
		if len(parts) >= 3 {
			bodyContent = parts[2]
			for _, line := range strings.Split(parts[1], "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "seal_") {
					continue
				}
				if trimmed != "" {
					newLines = append(newLines, line)
				}
			}
		} else {
			// No YAML frontmatter yet — seal the whole file as the body.
			bodyContent = content
			if !strings.HasPrefix(bodyContent, "\n") {
				bodyContent = "\n" + bodyContent
			}
		}

		if version == "" {
			version = "1.0.0"
		}
		if issuer == "" {
			issuer = "system:cli"
		}
		sealHeader := skill.GenerateSealData(bodyContent, version, issuer, time.Now())
		if len(newLines) > 0 {
			sealHeader = strings.Join(newLines, "\n") + "\n" + sealHeader
		}
		skillContent := "---\n" + sealHeader + "\n---" + bodyContent

		if err := fileutil.WriteSecureFile(skillPath, []byte(skillContent)); err != nil {
			return errfmt.Newf("save sealed skill").Wrap(err)
		}

		logger.Info("Successfully sealed skill", logging.String("skill", skillName), logging.String("path", skillPath))
		return cli.FormatOutput(cmd, map[string]any{
			"skill":                 skillName,
			objects.FieldKeyPath:    skillPath,
			objects.FieldKeyVersion: version,
			"issuer":                issuer,
			objects.FieldKeyStatus:  "sealed",
		})
	})(cmd, args)
}
