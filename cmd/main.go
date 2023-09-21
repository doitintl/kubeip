package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/doitintl/kubeip/internal/config"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	version   string
	buildDate string
	gitCommit string
	gitBranch string
)

func prepareLogger(level string, json bool) *logrus.Entry {
	logger := logrus.New()

	// set debug log level
	switch level {
	case "debug", "DEBUG":
		logger.SetLevel(logrus.DebugLevel)
	case "info", "INFO":
		logger.SetLevel(logrus.InfoLevel)
	case "warning", "WARNING":
		logger.SetLevel(logrus.WarnLevel)
	case "error", "ERROR":
		logger.SetLevel(logrus.ErrorLevel)
	case "fatal", "FATAL":
		logger.SetLevel(logrus.FatalLevel)
	case "panic", "PANIC":
		logger.SetLevel(logrus.PanicLevel)
	default:
		logger.SetLevel(logrus.WarnLevel)
	}

	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if json {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

	log := logger.WithFields(logrus.Fields{
		"version": version,
	})

	return log
}

func run(_ context.Context, _ *logrus.Entry, _ config.Config) error {
	return nil
}

func runCmd(c *cli.Context) error {
	ctx := signals.SetupSignalHandler()
	log := prepareLogger(c.String("log-level"), c.Bool("json"))
	cfg := config.LoadConfig(c)

	if err := run(ctx, log, cfg); err != nil {
		log.Fatalf("eks-lens agent failed: %v", err)
	}

	return nil
}

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "run agent",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "cluster-name",
						Usage:    "Kubernetes cluster name (not needed if running in cluster)",
						EnvVars:  []string{"CLUSTER_NAME"},
						Category: "Configuration",
					},
					&cli.PathFlag{
						Name:     "kubeconfig",
						Usage:    "Path to Kubernetes configuration file (not needed if running in cluster)",
						EnvVars:  []string{"KUBECONFIG"},
						Category: "Configuration",
					},
					&cli.StringFlag{
						Name:     "log-level",
						Usage:    "set log level (debug, info(*), warning, error, fatal, panic)",
						Value:    "info",
						EnvVars:  []string{"LOG_LEVEL"},
						Category: "Logging",
					},
					&cli.BoolFlag{
						Name:     "json",
						Usage:    "produce log in JSON format: Logstash and Splunk friendly",
						EnvVars:  []string{"LOG_JSON"},
						Category: "Logging",
					},
					&cli.BoolFlag{
						Name:     "develop-mode",
						Usage:    "enable develop mode",
						EnvVars:  []string{"DEV_MODE"},
						Category: "Development",
					},
				},
				Action: runCmd,
			},
		},
		Name:    "kubeip-agent",
		Usage:   "replaces the node's public IP address with a static public IP address",
		Version: version,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("kubeip-agent %s\n", version)
		fmt.Printf("  Build date: %s\n", buildDate)
		fmt.Printf("  Git commit: %s\n", gitCommit)
		fmt.Printf("  Git branch: %s\n", gitBranch)
		fmt.Printf("  Built with: %s\n", runtime.Version())
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}
}
