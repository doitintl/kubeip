package main

import (
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/controller"
	"github.com/doitintl/kubeip/internal/kipcompute"
	"github.com/sirupsen/logrus"
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

func main() {
	ctx := signals.SetupSignalHandler()
	cfg := config.NewConfig()
	logger := prepareLogger(cfg.LogLevel, cfg.LogJSON)
	logger.WithField("config", cfg).Info("using kubeIP configuration")

	cluster, err := kipcompute.ClusterName()
	if err != nil {
		logger.WithError(err).Fatal("failed to get cluster name")
	}

	project, err := kipcompute.ProjectName()
	if err != nil {
		logger.WithError(err).Fatal("failed to get project name")
	}

	logger = logger.WithFields(logrus.Fields{
		"cluster":   cluster,
		"project":   project,
		"branch":    gitBranch,
		"commit":    gitCommit,
		"buildDate": buildDate,
	})
	logger.Info("starting kubeIP controller")

	if err = controller.Start(ctx, logger, project, cluster, cfg); err != nil {
		logrus.WithError(err).Fatal("failed to start kubeIP controller")
	}
}
