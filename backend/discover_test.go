package main

import "testing"

func overrideDiscoverSource(t *testing.T, template string) {
	t.Helper()
	previous := feedKnownSources
	feedKnownSources = []discoverSource{{ID: "custom", Template: template}}
	t.Cleanup(func() { feedKnownSources = previous })
}
