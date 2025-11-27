package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestTicketSetGetDelete(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", oldHome)

	viper.Reset()
	t.Cleanup(viper.Reset)
	ticketsCache = nil

	if err := setTicket("Acme", "Web", "ABC-1"); err != nil {
		t.Fatalf("setTicket failed: %v", err)
	}
	if val, ok := getTicket("Acme", "Web"); !ok || val != "ABC-1" {
		t.Fatalf("getTicket returned %v, %v", val, ok)
	}
	if err := deleteTicket("Acme", "Web"); err != nil {
		t.Fatalf("deleteTicket failed: %v", err)
	}
	if _, ok := getTicket("Acme", "Web"); ok {
		t.Fatalf("expected ticket to be removed")
	}
	if err := deleteTicket("Acme", "Web"); err == nil {
		t.Fatalf("expected delete of missing ticket to error")
	}
}

func TestTicketListCommand(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", oldHome)

	viper.Reset()
	t.Cleanup(viper.Reset)
	ticketsCache = nil

	if err := setTicket("Acme", "Web", "ABC-1"); err != nil {
		t.Fatalf("setTicket failed: %v", err)
	}
	if err := setTicket("Beta", "App", "XYZ-2"); err != nil {
		t.Fatalf("setTicket failed: %v", err)
	}

	buf := &bytes.Buffer{}
	ticketListCmd.SetOut(buf)
	ticketListCmd.SetErr(buf)
	ticketListCustomer = ""
	if ticketListCmd.RunE != nil {
		if err := ticketListCmd.RunE(ticketListCmd, nil); err != nil {
			t.Fatalf("ticket list failed: %v", err)
		}
	} else {
		ticketListCmd.Run(ticketListCmd, nil)
	}
	out := buf.String()
	if !strings.Contains(out, "Acme / Web: ABC-1") || !strings.Contains(out, "Beta / App: XYZ-2") {
		t.Fatalf("unexpected list output: %s", out)
	}
}
