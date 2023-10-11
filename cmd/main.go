package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	nd "github.com/doitintl/kubeip/internal/node"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

type contextKey string

const (
	developModeKey contextKey = "develop-mode"
)

var (
	version      = "dev"
	buildDate    string
	gitCommit    string
	gitBranch    string
	errEmptyPath = errors.New("empty path")
)

const (
	// DefaultRetryInterval is the default retry interval
	defaultRetryInterval = 5 * time.Minute
	defaultRetryAttempts = 10
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

	// record the file name and line number of the log
	logger.SetReportCaller(true)

	log := logger.WithFields(logrus.Fields{
		"version": version,
	})

	return log
}

func assignAddress(c context.Context, log *logrus.Entry, assigner address.Assigner, node *types.Node, cfg *config.Config) error {
	ctx, cancel := context.WithCancel(c)
	defer cancel()
	// retry counter
	retryCounter := 0
	// ticker for retry interval
	ticker := time.NewTicker(cfg.RetryInterval)
	defer ticker.Stop()

	for {
		err := assigner.Assign(ctx, node.Instance, node.Zone, cfg.Filter, cfg.OrderBy)
		if err != nil {
			log.WithError(err).Errorf("failed to assign static public IP address to node %s", node.Name)
			if retryCounter < cfg.RetryAttempts {
				retryCounter++
				log.Infof("retrying after %v", cfg.RetryInterval)
			} else {
				log.Infof("reached maximum number of retries (%d)", cfg.RetryAttempts)
				return errors.Wrap(err, "reached maximum number of retries")
			}
			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				log.Infof("kubeip agent stopped")
				return errors.Wrap(err, "context is done")
			}
		}
		break
	}
	return nil
}

func run(c context.Context, log *logrus.Entry, cfg *config.Config) error {
	ctx, cancel := context.WithCancel(c)
	defer cancel()
	// add debug mode to context
	if cfg.DevelopMode {
		ctx = context.WithValue(ctx, developModeKey, true)
	}
	log.WithField("develop-mode", cfg.DevelopMode).Infof("kubeip agent started")

	restconfig, err := retrieveKubeConfig(log, cfg)
	if err != nil {
		return errors.Wrap(err, "retrieving kube config")
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return errors.Wrap(err, "initializing kubernetes client")
	}

	explorer := nd.NewExplorer(clientset)
	n, err := explorer.GetNode(ctx, cfg.NodeName)
	if err != nil {
		return errors.Wrap(err, "getting node")
	}

	// assign static public IP address with retry (interval and attempts)
	assigner, err := address.NewAssigner(ctx, log, n.Cloud, cfg)
	if err != nil {
		return errors.Wrap(err, "initializing assigner")
	}
	// assign static public IP address
	errorCh := make(chan error)
	go func() {
		e := assignAddress(ctx, log, assigner, n, cfg)
		if e != nil {
			errorCh <- e
		}
	}()

	select {
	case err = <-errorCh:
		if err != nil {
			return errors.Wrap(err, "assigning static public IP address")
		}
	case <-ctx.Done():
		log.Infof("kubeip agent stopped")
	}

	return nil
}

func runCmd(c *cli.Context) error {
	ctx := signals.SetupSignalHandler()
	log := prepareLogger(c.String("log-level"), c.Bool("json"))
	cfg := config.NewConfig(c)

	if err := run(ctx, log, cfg); err != nil {
		log.Fatalf("eks-lens agent failed: %v", err)
	}

	return nil
}

func main() {
	app := &cli.App{
		// use ";" instead of "," for slice flag separator
		// AWS filter values can contain "," and shorthand filter format uses "," to separate Names and Values
		SliceFlagSeparator: ";",
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "run agent",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "node-name",
						Usage:    "Kubernetes node name (not needed if running in node)",
						EnvVars:  []string{"NODE_NAME"},
						Category: "Configuration",
					},
					&cli.StringFlag{
						Name:     "project",
						Usage:    "name of the GCP project or the AWS account ID (not needed if running in node)",
						EnvVars:  []string{"PROJECT"},
						Category: "Configuration",
					},
					&cli.StringFlag{
						Name:     "region",
						Usage:    "name of the GCP region or the AWS region (not needed if running in node)",
						EnvVars:  []string{"REGION"},
						Category: "Configuration",
					},
					&cli.PathFlag{
						Name:     "kubeconfig",
						Usage:    "path to Kubernetes configuration file (not needed if running in node)",
						EnvVars:  []string{"KUBECONFIG"},
						Category: "Configuration",
					},
					&cli.DurationFlag{
						Name:     "retry-interval",
						Usage:    "when the agent fails to assign the static public IP address, it will retry after this interval",
						Value:    defaultRetryInterval,
						EnvVars:  []string{"RETRY_INTERVAL"},
						Category: "Configuration",
					},
					&cli.StringSliceFlag{
						Name:     "filter",
						Usage:    "filter for the IP addresses",
						EnvVars:  []string{"FILTER"},
						Category: "Configuration",
					},
					&cli.StringFlag{
						Name:     "order-by",
						Usage:    "order by for the IP addresses",
						EnvVars:  []string{"ORDER_BY"},
						Category: "Configuration",
					},
					&cli.IntFlag{
						Name:     "retry-attempts",
						Usage:    "number of attempts to assign the static public IP address",
						Value:    defaultRetryAttempts,
						EnvVars:  []string{"RETRY_ATTEMPTS"},
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

func kubeConfigFromPath(kubepath string) (*rest.Config, error) {
	if kubepath == "" {
		return nil, errEmptyPath
	}

	data, err := os.ReadFile(kubepath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading kubeconfig at %s", kubepath)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, errors.Wrapf(err, "building rest config from kubeconfig at %s", kubepath)
	}

	return cfg, nil
}

func retrieveKubeConfig(log logrus.FieldLogger, cfg *config.Config) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromPath(cfg.KubeConfigPath)
	if err != nil && !errors.Is(err, errEmptyPath) {
		return nil, errors.Wrap(err, "retrieving kube config from path")
	}

	if kubeconfig != nil {
		log.Debug("using kube config from env variables")
		return kubeconfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving in node kube config")
	}
	log.Debug("using in node kube config")
	return inClusterConfig, nil
}
