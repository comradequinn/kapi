package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/comradequinn/kapi"
)

var (
	Version  = "1.0.0"
	Hostname = func() string {
		h, _ := os.Hostname()
		return h
	}()
)

func main() {
	var (
		ctx, _ = signal.NotifyContext(context.Background(), syscall.SIGTERM)
		obs    kapi.ObservabilityConfig
	)

	defer func() {
		if err := recover(); err != nil {
			obs.LogFunc(ctx, 0, "panic occurred. terminating", "error", err)
			os.Exit(1)
		}
	}()

	obs = kapi.UseSlog(ctx, slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})).With("app", "kapi-example", "version", Version))

	kapi.Init(obs)

	k, err := kapi.NewCluster(ctx, kapi.ClusterConfig{
		Namespaces: []string{"kapi-example"},
		CRDs: []kapi.CRDs{
			{
				APIGroup:   "kapi-example.comradequinn.github.io",
				APIVersion: "v1",
				Kinds: map[string]kapi.KindType{
					"ConfigAudit":     &ConfigAudit{},
					"ConfigAuditList": &ConfigAuditList{},
				},
			},
		},
	})

	if err != nil {
		panic(err)
	}

	if err := addReconcilerExample(ctx, k); err != nil {
		panic(err)
	}

	go func() {
		if err := k.Connect(ctx); err != nil {
			obs.LogFunc(ctx, 0, fmt.Sprintf("error connecting to k8s. %v", err))
			os.Exit(1)
		}
	}()

	obs.LogFunc(ctx, 2, "intialised")

	<-ctx.Done()
	obs.LogFunc(ctx, 2, "sigterm received. terminating")
	time.Sleep(time.Second) // allow time for logs to flush
}
