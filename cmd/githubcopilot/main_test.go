package main

import (
	"reflect"
	"testing"
)

func TestSplitLaunchArgsAllowsFlagsAfterTarget(t *testing.T) {
	flags, target := splitLaunchArgs([]string{"codex-app", "--model", "gpt-5.4", "--server-only"})
	if !reflect.DeepEqual(target, []string{"codex-app"}) {
		t.Fatalf("unexpected target args: %#v", target)
	}
	if !reflect.DeepEqual(flags, []string{"--model", "gpt-5.4", "--server-only"}) {
		t.Fatalf("unexpected flag args: %#v", flags)
	}
}

func TestSplitLaunchArgsAllowsMultiwordTarget(t *testing.T) {
	flags, target := splitLaunchArgs([]string{"--port=11436", "codex", "app", "--no-launch"})
	if !reflect.DeepEqual(target, []string{"codex", "app"}) {
		t.Fatalf("unexpected target args: %#v", target)
	}
	if !reflect.DeepEqual(flags, []string{"--port=11436", "--no-launch"}) {
		t.Fatalf("unexpected flag args: %#v", flags)
	}
}
