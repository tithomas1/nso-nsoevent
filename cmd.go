/*
Author:  Tim Thomas
Created: 28-Sep-2020
*/

package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"net/http"
	_ "net/http/pprof"
)

func newBaseCmd() *cobra.Command {

	cobra.OnInitialize(initConfig)

	// Start with the base command

	baseCmd := &cobra.Command{
		Use:          programName,
		Version:      programVersion,
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			_ = server.startSubscribers()
			return nil
		},
	}

	// Global flags across all commands

	baseCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose logging")
	_ = viper.BindPFlag("verbose", baseCmd.PersistentFlags().Lookup("verbose"))
	baseCmd.PersistentFlags().BoolP("debug", "d", false, "enable debug output")
	_ = viper.BindPFlag("debug", baseCmd.PersistentFlags().Lookup("debug"))
	baseCmd.PersistentFlags().BoolP("nocolor", "", false, "disable colorized output")
	_ = viper.BindPFlag("nocolor", baseCmd.PersistentFlags().Lookup("nocolor"))

	baseCmd.PersistentFlags().Int("pprofPort", 0, "listen port for pprof server")
	_ = viper.BindPFlag("pprofPort", baseCmd.PersistentFlags().Lookup("pprofPort"))

	baseCmd.PersistentFlags().StringP("user", "u", defaultNSOUser, "user for NSO API")
	_ = viper.BindPFlag("nso.user", baseCmd.PersistentFlags().Lookup("user"))
	baseCmd.PersistentFlags().StringP("password", "p", defaultNSOPassword, "password for NSO API")
	_ = viper.BindPFlag("nso.password", baseCmd.PersistentFlags().Lookup("password"))

	baseCmd.PersistentFlags().String("url", "", "NSO API URL (http://IP:PORT)")
	_ = viper.BindPFlag("nso.restconfAPI", baseCmd.PersistentFlags().Lookup("url"))

	baseCmd.PersistentFlags().DurationP("timeout", "t", defaultReadTime, "API timeout")
	_ = viper.BindPFlag("nso.readTimeout", baseCmd.PersistentFlags().Lookup("timeout"))

	// Subcommands

	cmdList := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list available event streams",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			server.printStreamList()
			return nil
		},
	}

	cmdInfo := &cobra.Command{
		Use:   "info",
		Short: "show server info",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			server.printState()
			return nil
		},
	}

	cmdInfoModels := &cobra.Command{
		Use:     "models",
		Aliases: []string{"m"},
		Short:   "show loaded data models",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			server.printModelList()
			return nil
		},
	}

	cmdInfoModels.PersistentFlags().BoolP("mounts", "m", false, "show mount detail")
	_ = viper.BindPFlag("mounts", cmdInfoModels.PersistentFlags().Lookup("mounts"))

	cmdInfo.AddCommand(cmdInfoModels)

	cmdInfoDatastores := &cobra.Command{
		Use:     "datastores",
		Aliases: []string{"data", "d"},
		Short:   "show datastores",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			server.printDatastoreList()
			return nil
		},
	}
	cmdInfo.AddCommand(cmdInfoDatastores)

	cmdInfoCallpoints := &cobra.Command{
		Use:     "callpoints",
		Aliases: []string{"call", "c"},
		Short:   "show callpoints",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			server.printCallpoints()
			return nil
		},
	}
	cmdInfo.AddCommand(cmdInfoCallpoints)

	cmdInfoActionpoints := &cobra.Command{
		Use:     "actionpoints",
		Aliases: []string{"action", "a"},
		Short:   "show actionpoints",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			server.printActionpoints()
			return nil
		},
	}
	cmdInfo.AddCommand(cmdInfoActionpoints)

	cmdInfoAPI := &cobra.Command{
		Use:   "api",
		Short: "show API data",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			err = server.printAPIData(args[0])
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmdInfo.AddCommand(cmdInfoAPI)

	cmdSubscribe := &cobra.Command{
		Use:     "subscribe",
		Aliases: []string{"sub", "listen"},
		Short:   "subscribe to one or more event streams",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return processConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := mainStartup()
			if err != nil {
				return err
			}
			fmt.Printf(stringColorize("### NSO event streams from %s\n", COLOR_HIGHLIGHT), Config.nsoTarget.apiUrl)
			server.validateWebhooks()
			if Config.debug {
				Config.webhooks.print()
			}
			_ = server.startSubscribers()
			return nil
		},
	}

	cmdSubscribe.PersistentFlags().StringSliceP("stream", "s", nil, "stream(s) to subscribe to")
	_ = viper.BindPFlag("stream", cmdSubscribe.PersistentFlags().Lookup("stream"))

	// Put all the commands together

	baseCmd.AddCommand(cmdList)
	baseCmd.AddCommand(cmdInfo)
	baseCmd.AddCommand(cmdSubscribe)

	return baseCmd
}

// Required initialization for all commands. Basically bring up the connection to
// the server and grab some basic info

func mainStartup() (*NsoServer, error) {
	// pprof profiling
	if Config.profilingPort != 0 {
		go func() {
			http.ListenAndServe(fmt.Sprintf(":%d", Config.profilingPort), nil)
		}()
	}

	// New NSO server
	server := newNSOServer()

	// Determine the root resource as presented by the server
	if err := server.getRootResource(); err != nil {
		return nil, err
	}

	// Retrieve info about the current server state
	if err := server.getState(); err != nil {
		return nil, err
	}

	// Retrieve the list of available streams from the server
	if err := server.getStreamList(); err != nil {
		return nil, err
	}
	return server, nil
}
