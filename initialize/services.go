package initialize

import (
	"fmt"
	"time"

	"github.com/quay/claircore/libindex"
	"github.com/quay/claircore/libvuln"
	"github.com/quay/claircore/libvuln/driver"
	"github.com/rs/zerolog"
	"gopkg.in/square/go-jose.v2/jwt"

	clairerror "github.com/quay/clair/v4/clair-error"
	"github.com/quay/clair/v4/config"
	"github.com/quay/clair/v4/httptransport"
	"github.com/quay/clair/v4/httptransport/client"
	notifier "github.com/quay/clair/v4/notifier/service"
)

const (
	// DefaultUpdatePeriod is the default period used for running updaters
	// within matcher processes.
	DefaultUpdatePeriod = 30 * time.Minute
	// NotifierIssuer is the value used for the issuer claim of any outgoing
	// HTTP requests the notifier makes, if PSK auth is configured.
	NotifierIssuer = `clair-notifier`
)

// Services will initialize the correct ClairCore services
// dependent on operation mode.
//
// Services maybe local or remote (over a network).
func (i *Init) Services() error {
	log := zerolog.Ctx(i.GlobalCTX).With().Str("component", "init/Init.Services").Logger()
	log.Info().Msg("begin service initialization")

	switch i.conf.Mode {
	case config.ComboMode:
		// configure two local services via claircore libraries
		opts := libindex.Opts{
			ConnString:           i.conf.Indexer.ConnString,
			ScanLockRetry:        time.Duration(i.conf.Indexer.ScanLockRetry) * time.Second,
			LayerScanConcurrency: i.conf.Indexer.LayerScanConcurrency,
			Migrations:           i.conf.Indexer.Migrations,
			Airgap:               i.conf.Indexer.Airgap,
		}
		if i.conf.Indexer.Scanner.Package != nil {
			opts.ScannerConfig.Package = make(map[string]func(interface{}) error, len(i.conf.Indexer.Scanner.Package))
			for name, node := range i.conf.Indexer.Scanner.Package {
				opts.ScannerConfig.Package[name] = node.Decode
			}
		}
		if i.conf.Indexer.Scanner.Dist != nil {
			opts.ScannerConfig.Dist = make(map[string]func(interface{}) error, len(i.conf.Indexer.Scanner.Dist))
			for name, node := range i.conf.Indexer.Scanner.Dist {
				opts.ScannerConfig.Dist[name] = node.Decode
			}
		}
		if i.conf.Indexer.Scanner.Repo != nil {
			opts.ScannerConfig.Repo = make(map[string]func(interface{}) error, len(i.conf.Indexer.Scanner.Repo))
			for name, node := range i.conf.Indexer.Scanner.Repo {
				opts.ScannerConfig.Repo[name] = node.Decode
			}
		}
		libI, err := libindex.New(i.GlobalCTX, &opts)
		if err != nil {
			return clairerror.ErrNotInitialized{Msg: "failed to initialize libindex: " + err.Error()}
		}
		updaterConfigs := make(map[string]driver.ConfigUnmarshaler)
		for name, node := range i.conf.Updaters.Config {
			updaterConfigs[name] = node.Decode
		}
		libV, err := libvuln.New(i.GlobalCTX, &libvuln.Opts{
			MaxConnPool:     int32(i.conf.Matcher.MaxConnPool),
			ConnString:      i.conf.Matcher.ConnString,
			Migrations:      i.conf.Matcher.Migrations,
			UpdaterSets:     i.conf.Updaters.Sets,
			UpdateInterval:  i.conf.Matcher.Period,
			UpdaterConfigs:  updaterConfigs,
			UpdateRetention: i.conf.Matcher.UpdateRetention,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize libvuln: %v", err)
		}

		c, _, err := i.conf.Client(nil, notifierClaim)
		if err != nil {
			return err
		}

		n, err := notifier.New(i.GlobalCTX, notifier.Opts{
			DeliveryInterval: i.conf.Notifier.DeliveryInterval,
			ConnString:       i.conf.Notifier.ConnString,
			Indexer:          libI,
			Matcher:          libV,
			Client:           c,
			Migrations:       i.conf.Notifier.Migrations,
			PollInterval:     i.conf.Notifier.PollInterval,
			DisableSummary:   i.conf.Notifier.DisableSummary,
			Webhook:          i.conf.Notifier.Webhook,
			AMQP:             i.conf.Notifier.AMQP,
			STOMP:            i.conf.Notifier.STOMP,
		})
		if err != nil {
			return &clairerror.ErrNotInitialized{
				Msg: "notifier failed to initialize: " + err.Error(),
			}
		}

		i.Indexer = libI
		i.Matcher = libV
		i.Notifier = n
	case config.IndexerMode:
		// configure just a local indexer
		opts := libindex.Opts{
			ConnString:           i.conf.Indexer.ConnString,
			ScanLockRetry:        time.Duration(i.conf.Indexer.ScanLockRetry) * time.Second,
			LayerScanConcurrency: i.conf.Indexer.LayerScanConcurrency,
			Migrations:           i.conf.Indexer.Migrations,
			Airgap:               i.conf.Indexer.Airgap,
		}
		if i.conf.Indexer.Scanner.Package != nil {
			opts.ScannerConfig.Package = make(map[string]func(interface{}) error, len(i.conf.Indexer.Scanner.Package))
			for name, node := range i.conf.Indexer.Scanner.Package {
				opts.ScannerConfig.Package[name] = node.Decode
			}
		}
		if i.conf.Indexer.Scanner.Dist != nil {
			opts.ScannerConfig.Dist = make(map[string]func(interface{}) error, len(i.conf.Indexer.Scanner.Dist))
			for name, node := range i.conf.Indexer.Scanner.Dist {
				opts.ScannerConfig.Dist[name] = node.Decode
			}
		}
		if i.conf.Indexer.Scanner.Repo != nil {
			opts.ScannerConfig.Repo = make(map[string]func(interface{}) error, len(i.conf.Indexer.Scanner.Repo))
			for name, node := range i.conf.Indexer.Scanner.Repo {
				opts.ScannerConfig.Repo[name] = node.Decode
			}
		}
		libI, err := libindex.New(i.GlobalCTX, &opts)
		if err != nil {
			return clairerror.ErrNotInitialized{Msg: "failed to initialize libindex: " + err.Error()}
		}
		i.Indexer = libI
		i.Matcher = nil
	case config.MatcherMode:
		updaterConfigs := make(map[string]driver.ConfigUnmarshaler)
		for name, node := range i.conf.Updaters.Config {
			updaterConfigs[name] = node.Decode
		}
		// configure a local matcher but a remote indexer
		libV, err := libvuln.New(i.GlobalCTX, &libvuln.Opts{
			MaxConnPool:     int32(i.conf.Matcher.MaxConnPool),
			ConnString:      i.conf.Matcher.ConnString,
			Migrations:      i.conf.Matcher.Migrations,
			UpdaterSets:     i.conf.Updaters.Sets,
			UpdateInterval:  i.conf.Matcher.Period,
			UpdaterConfigs:  updaterConfigs,
			UpdateRetention: i.conf.Matcher.UpdateRetention,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize libvuln: %v", err)
		}
		// matcher mode needs a remote indexer client
		c, auth, err := i.conf.Client(nil, intraserviceClaim)
		switch {
		case err != nil:
			return err
		case !auth && i.conf.Auth.Any():
			return &clairerror.ErrNotInitialized{
				Msg: "client authorization required but not provided",
			}
		default: // OK
		}
		remoteIndexer, err := client.NewHTTP(i.GlobalCTX,
			client.WithAddr(i.conf.Matcher.IndexerAddr),
			client.WithClient(c))
		if err != nil {
			return err
		}
		i.Indexer = remoteIndexer
		i.Matcher = libV
	case config.NotifierMode:
		// notifier uses a remote indexer and matcher
		c, auth, err := i.conf.Client(nil, intraserviceClaim)
		switch {
		case err != nil:
			return err
		case !auth && i.conf.Auth.Any():
			return &clairerror.ErrNotInitialized{
				Msg: "client authorization required but not provided",
			}
		default: // OK
		}

		remoteIndexer, err := client.NewHTTP(i.GlobalCTX,
			client.WithAddr(i.conf.Notifier.IndexerAddr),
			client.WithClient(c))
		if err != nil {
			return err
		}

		remoteMatcher, err := client.NewHTTP(i.GlobalCTX,
			client.WithAddr(i.conf.Notifier.MatcherAddr),
			client.WithClient(c))
		if err != nil {
			return err
		}

		c, _, err = i.conf.Client(nil, notifierClaim)
		if err != nil {
			return err
		}

		n, err := notifier.New(i.GlobalCTX, notifier.Opts{
			DeliveryInterval: i.conf.Notifier.DeliveryInterval,
			ConnString:       i.conf.Notifier.ConnString,
			Indexer:          remoteIndexer,
			Matcher:          remoteMatcher,
			Client:           c,
			Migrations:       i.conf.Notifier.Migrations,
			PollInterval:     i.conf.Notifier.PollInterval,
			Webhook:          i.conf.Notifier.Webhook,
			AMQP:             i.conf.Notifier.AMQP,
			STOMP:            i.conf.Notifier.STOMP,
		})
		if err != nil {
			return &clairerror.ErrNotInitialized{
				Msg: "notifier failed to initialize: " + err.Error(),
			}
		}
		i.Indexer = remoteIndexer
		i.Matcher = remoteMatcher
		i.Notifier = n

	default:
		return fmt.Errorf("could not determine passed in mode: %v", i.conf.Mode)
	}

	return nil
}

var (
	intraserviceClaim = jwt.Claims{Issuer: httptransport.IntraserviceIssuer}
	notifierClaim     = jwt.Claims{Issuer: NotifierIssuer}
)
