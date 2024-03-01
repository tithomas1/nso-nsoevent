/*
Author:  Tim Thomas
Created: 21-Sep-2020
*/

package main

import (
	"fmt"
	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"
	"os"
	"regexp"
	"strconv"
	"time"
)

const (
	defaultConfigFile   = programName + "Config"
	outputColumnPadding = 2
	defaultNSOAddress   = "127.0.0.1"
	defaultNSOPort      = 8080
	defaultConnectTime  = 3 * time.Second
	defaultReadTime     = 10 * time.Minute
	defaultNSOUser      = "admin"
	defaultNSOPassword  = "admin"
	defaultWebhookUser  = "netgitops"
)

var restconfApiRE = regexp.MustCompile("http(s?)://([0-9A-Za-z.]*):?([0-9]*)?")

type nsoInfo struct {
	cmdUrl    string
	apiUrl    string
	ipAddress string
	port      int
	user      string
	password  string
}

var Config struct {
	verbose        bool
	debug          bool
	noColor        bool
	profilingPort  int
	showMounts     bool
	nsoTarget      nsoInfo
	connectTimeout time.Duration
	readTimeout    time.Duration
	streamNames    []string
	webhooks       webhooks
}

func initConfig() {

	// Look for any environment variables prefixed with "NSOEVENT_"

	viper.SetEnvPrefix("NSOEVENT")
	viper.AutomaticEnv()

	viper.SetDefault("nso.connectTimeout", defaultConnectTime)
	viper.SetDefault("nso.readTimeout", defaultReadTime)
	viper.SetDefault("nso.user", defaultNSOUser)
	viper.SetDefault("nso.password", defaultNSOPassword)
	viper.SetDefault("nso.restconfAPI", fmt.Sprintf("http://%s:%d", defaultNSOAddress, defaultNSOPort))

	viper.SetConfigName(defaultConfigFile)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/." + programName)
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			panic(fmt.Errorf("(initConfig) fatal error accessing config file: %v", err))
		}
	}
}

func processConfig() error {
	// Global settings & flags
	Config.verbose = viper.GetBool("verbose")
	Config.debug = viper.GetBool("debug")
	Config.noColor = viper.GetBool("nocolor")
	Config.profilingPort = viper.GetInt("pprofPort")
	Config.nsoTarget.cmdUrl = viper.GetString("nso.restconfAPI")
	Config.nsoTarget.user = viper.GetString("nso.user")
	Config.nsoTarget.password = viper.GetString("nso.password")
	Config.connectTimeout = viper.GetDuration("nso.connectTimeout")
	Config.readTimeout = viper.GetDuration("nso.readTimeout")

	// info models command
	Config.showMounts = viper.GetBool("mounts")

	// Subscribe command
	Config.streamNames = viper.GetStringSlice("stream")

	// Override the color setting if trying to do color with something that can't
	if os.Getenv("TERM") == "dumb" || (!isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())) {
		Config.noColor = true
	}

	// Parse the NSO URL
	if Config.nsoTarget.cmdUrl == "" {
		return fmt.Errorf("(processConfig) NSO API URL not found")
	}

	subMatches := restconfApiRE.FindStringSubmatch(Config.nsoTarget.cmdUrl)
	if len(subMatches) < 3 {
		return fmt.Errorf("(processConfig) cannot parse NSO API URL '%s'", Config.nsoTarget.cmdUrl)
	}

	// TLS?
	protocol := "http"
	if subMatches[1] != "" {
		protocol = "https"
		fmt.Println(stringColorize("WARNING: TLS protocol detected, but insecure verify is true!", COLOR_WARNING))
	}

	Config.nsoTarget.ipAddress = defaultNSOAddress
	if subMatches[2] != "" {
		Config.nsoTarget.ipAddress = subMatches[2]
	}

	Config.nsoTarget.port = defaultNSOPort
	if len(subMatches) == 4 && subMatches[3] != "" {
		port, err := strconv.Atoi(subMatches[3])
		if err != nil {
			return fmt.Errorf("(processConfig) invalid NSO API port in '%s'", subMatches[0])
		}
		Config.nsoTarget.port = port
	}

	Config.nsoTarget.apiUrl = fmt.Sprintf("%s://%s:%d", protocol, Config.nsoTarget.ipAddress, Config.nsoTarget.port)

	if Config.debug {
		enableDebug()
	}

	// Webhook definitions

	if err := viper.UnmarshalKey("webhooks", &Config.webhooks); err != nil {
		return fmt.Errorf("(processConfig) fatal error processing config file for 'webhooks' key: %v", err)
	}

	// Initial validation of webhook definitions
	if hookCount := len(Config.webhooks); hookCount > 0 {
		for i, hook := range Config.webhooks {
			if hook.Stream == "" {
				return fmt.Errorf("(processConfig) fatal error in config file: webhook %d - missing stream name", i+1)
			}
			if hook.Url == "" {
				return fmt.Errorf("(processConfig) fatal error in config file: webhook %d - missing target URL", i+1)
			}
			if hook.User != "" && hook.ApiToken == "" {
				return fmt.Errorf("(processConfig) fatal error in config file: webhook %d - missing API token for user %s@%s", i+1, hook.User, hook.Url)
			}
			if hook.User == "" && hook.ApiToken == "" {
				Config.webhooks[i].User = defaultWebhookUser
			}
		}
	}

	return nil
}
