package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/nats-io/go-nats"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()
	flagSet := flag.NewFlagSet("config", flag.ExitOnError)
	addrStr := flagSet.String("addr", "nats://localhost:4222", "comma separated list of nats server addresses (default localhost:4222)")
	streamName := flagSet.String("subject", "webhooks", "liftbridge stream to publish webhooks to. (default: webhooks")
	credsPath := flagSet.String("creds-path", "/nats/sys.creds", "path to nats credentials file")
	err := flagSet.Parse(os.Args[1:])

	if err != nil {
		log.Fatal(err)
	}

	if len(*addrStr) == 0 {
		log.Fatal("param addr cannot be empty")
	}

	if len(*streamName) == 0 {
		log.Fatal("param subject cannot be empty")
	}

	if len(*credsPath) == 0 {
		log.Fatal("param nats creds cannot be empty")
	}

	addr := strings.Split(*addrStr, ",")
	client, err := newConn(addr, *credsPath)

	if err != nil {
		log.Fatalf("error creating liftbridge %v\n", err)
	}

	mux := chi.NewRouter()
	mux.Post("/", handleHook(client, *streamName, sugar))

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := chi.ServerBaseContext(baseCtx, mux)

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		_ = <-sigs
		sugar.Infow("received shutdown signal")
		cancel()
		select {
		case <-baseCtx.Done():
		case <-time.After(10 * time.Second):
			sugar.Errorw("base context did not finish in time")
		}
		done <- true
	}()

	go func() {
		sugar.Infow("listening", "port", "8080")
		err = http.ListenAndServe(":8080", h)
		if err != nil {
			sugar.Fatal(err)
		}
	}()
	<-done
	sugar.Infow("received shutdown signal")
}

func newConn(addr []string, path string) (*nats.Conn, error) {
	opts := nats.GetDefaultOptions()
	opts.Servers = addr

	err := nats.UserCredentials(path)(&opts)

	if err != nil {
		fmt.Errorf("nats user credentials opts fail: %v", err)
	}

	return opts.Connect()
}

func handleHook(conn *nats.Conn, subject string, logger *zap.SugaredLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Infow("webhook received")
		defer r.Body.Close()
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Infow("unable to read hook body", "err", err)
			return
		}

		if len(b) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}

		err = conn.Publish(subject, b)

		if err != nil {
			logger.Infow("unable to publish message", "err", err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
