package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	barrycli "github.com/go-barry/barry/cli"
	"github.com/urfave/cli/v2"
)

func dummyCmd(name string) *cli.Command {
	return &cli.Command{
		Name: name,
		Action: func(c *cli.Context) error {
			return nil
		},
	}
}

func failingCmd(name string) *cli.Command {
	return &cli.Command{
		Name: name,
		Action: func(c *cli.Context) error {
			return errors.New("intentional failure")
		},
	}
}

func Test_runApp_SuccessfulCommands(t *testing.T) {
	barrycli.InitCommand = dummyCmd("init")
	barrycli.DevCommand = dummyCmd("dev")
	barrycli.ProdCommand = dummyCmd("prod")
	barrycli.CleanCommand = dummyCmd("clean")
	barrycli.CheckCommand = dummyCmd("check")
	barrycli.InfoCommand = dummyCmd("info")

	commands := []string{"init", "dev", "prod", "clean", "check", "info"}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			err := runApp([]string{"barry", cmd})
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
		})
	}
}

func Test_runApp_ErrorCommand(t *testing.T) {
	barrycli.InitCommand = failingCmd("init")
	barrycli.DevCommand = dummyCmd("dev")
	barrycli.ProdCommand = dummyCmd("prod")
	barrycli.CleanCommand = dummyCmd("clean")
	barrycli.CheckCommand = dummyCmd("check")
	barrycli.InfoCommand = dummyCmd("info")

	err := runApp([]string{"barry", "init"})
	if err == nil || err.Error() != "intentional failure" {
		t.Fatalf("Expected error 'intentional failure', got: %v", err)
	}
}

func Test_main_LogFatalPath(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "invalidCommand")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")

	output, err := cmd.CombinedOutput()

	if exitErr, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("Expected exit error, got: %v", err)
	} else if exitErr.ExitCode() == 0 {
		t.Fatalf("Expected non-zero exit code from main")
	}

	if !strings.Contains(string(output), "No help topic for") {
		t.Errorf("Expected CLI error output, got: %s", output)
	}
}
