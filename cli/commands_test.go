package cli

import (
	"testing"

	"github.com/go-barry/barry"
	"github.com/urfave/cli/v2"
)

var recordedConfig *barry.RuntimeConfig

func mockStart(cfg barry.RuntimeConfig) {
	recordedConfig = &cfg
}

func TestDevCommand_UsesDevConfig(t *testing.T) {
	original := barry.Start
	barry.Start = mockStart
	t.Cleanup(func() {
		barry.Start = original
		recordedConfig = nil
	})

	app := &cli.App{Commands: []*cli.Command{DevCommand}}

	err := app.Run([]string{"barry", "dev"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if recordedConfig == nil {
		t.Fatal("expected Start to be called, but it was not")
	}

	if recordedConfig.Env != "dev" || recordedConfig.EnableCache != false || recordedConfig.Port != 8080 {
		t.Errorf("unexpected dev config: %+v", recordedConfig)
	}
}

func TestProdCommand_UsesProdConfig(t *testing.T) {
	original := barry.Start
	barry.Start = mockStart
	t.Cleanup(func() {
		barry.Start = original
		recordedConfig = nil
	})

	app := &cli.App{Commands: []*cli.Command{ProdCommand}}

	err := app.Run([]string{"barry", "prod"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if recordedConfig == nil {
		t.Fatal("expected Start to be called, but it was not")
	}

	if recordedConfig.Env != "prod" || recordedConfig.EnableCache != true || recordedConfig.Port != 8080 {
		t.Errorf("unexpected prod config: %+v", recordedConfig)
	}
}
