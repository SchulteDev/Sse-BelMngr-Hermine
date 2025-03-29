package cli

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const shortCommandDescription = "Sse-BelMngr-Hermine analyzes local documents " +
	"using Azure® AI and imports the data into the BelegManager of SteuerSparErklärung®"

const defaultBelegManagerImportPath = "BelegManager-Import"

var cmdConfigFile string

var Command = &cobra.Command{
	Short:        shortCommandDescription,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if initErr := initConfiguration(); initErr != nil {
			return initErr
		}
		if bindErr := bindFlags(cmd); bindErr != nil {
			return bindErr
		}

		return nil
	},
	PreRunE: validateCliArguments,
	RunE:    run,
}

func init() {
	Command.PersistentFlags().StringVarP(&cmdConfigFile, "config", "c", "", "Config file path")

	err := createApplicationFlags()
	if err != nil {
		panic(fmt.Errorf("not able to create flag: %w", err))
	}

	createLoggingFlags()
}

func createApplicationFlags() error {
	if err := createBelegManagerFlags(); err != nil {
		return err
	}

	persistentFlags := Command.PersistentFlags()

	persistentFlags.StringVar(&diKeyCliArgument, "di-key", "", "Azure AI Document Intelligence key")
	if err := Command.MarkPersistentFlagRequired("di-key"); err != nil {
		return err
	}

	persistentFlags.StringVar(&diEndpointCliArgument, "di-endpoint", "", "Azure AI Document Intelligence endpoint")
	if err := Command.MarkPersistentFlagRequired("di-endpoint"); err != nil {
		return err
	}

	return nil
}

func createBelegManagerFlags() error {
	currentUser, err := user.Current()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error getting current user: ", err)
		return err
	}
	userDocumentsDir := filepath.Join(currentUser.HomeDir, "Documents")

	/*
		   supportedFileTypes specifies the file types which can be imported
			- belegManager supports jpg, tiff, bmp, png, gif, xpm, tif, pdf
			- document intelligence supports jfif, pjp, jpg, pjepg, jepg, pdf, png, tif, tiff
	*/
	supportedFileTypes := []string{"jpg", "JPG", "pdf", "PDF", "png", "PNG", "tif", "TIF", "tiff", "TIFF"}
	supportedFileTypesAsGlob := "*.{" + strings.Join(supportedFileTypes, ",") + "}"
	filesToImportDefaultGlob :=
		filepath.Join(userDocumentsDir, defaultBelegManagerImportPath, "**", supportedFileTypesAsGlob)
	Command.
		PersistentFlags().
		StringVarP(
			&filesToImportGlobCliArgument,
			"files-to-import-glob",
			"f",
			filesToImportDefaultGlob,
			"Glob pattern identifying the documents to import",
		)
	viper.SetDefault("files-to-import-glob", filesToImportDefaultGlob)

	belegManagerHome := filepath.Join(userDocumentsDir, "BelegManager-Daten")
	Command.
		PersistentFlags().
		StringVar(
			&belegManagerDirectoryCliArgument,
			"beleg-manager-data-directory",
			belegManagerHome,
			"Path to the BelegManager data",
		)
	viper.SetDefault("beleg-manager-data-directory", belegManagerDirectoryCliArgument)

	return nil
}

func createLoggingFlags() {
	persistentFlags := Command.PersistentFlags()

	persistentFlags.StringVarP(
		&logLevelCliArgument,
		"log-level",
		"l",
		"info",
		"Log level (trace, debug, info, warn, error, fatal, panic)",
	)
	viper.SetDefault("log-level", logrus.InfoLevel)
}

func initConfiguration() error {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	if cmdConfigFile == "" {
		return nil
	}
	viper.SetConfigFile(cmdConfigFile)

	err := viper.ReadInConfig()
	if err == nil {
		fmt.Println("Using config file", viper.ConfigFileUsed())
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "Config file", viper.ConfigFileUsed(), "not read: ", err)
	}

	return nil
}

// Config value can be set via ENV, configFile or as command line argument.
func bindFlags(cmd *cobra.Command) error {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = viper.BindPFlag(f.Name, cmd.PersistentFlags().Lookup(f.Name))

		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				return
			}
		}
	})

	return nil
}
