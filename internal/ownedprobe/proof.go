package ownedprobe

import (
	"encoding/json"
	"io"
)

// ProofSystem is optional: existing System implementations remain valid. Run
// invokes it only after all existing observe guards, including EROFS and owned
// scratch cleanup, succeed. Implementations must independently guard every
// native effect; this interface is not an authorization capability.
//
// The six observations are fixed, not a general-purpose command or path API.
// There is deliberately no string-valued result that could leak system errors,
// environment values, child output or credentials into the public wire.
type ProofSystem interface {
	ObserveProof() ProofObservations
}

type ProofObservations struct {
	ScratchLimit, NetworkUnreachable, HostCredentialsAbsent bool
	CPU, Memory, Pids                                       CounterObservation
}

// Supported means both the owned operation and its cleanup/wait completed as
// required. A declared limit alone is never an observed counter effect.
type CounterObservation struct {
	Supported     bool
	Before, After uint64
}

type proofCheck struct {
	Name   string  `json:"name"`
	State  string  `json:"state"`
	Detail string  `json:"detail"`
	Before *uint64 `json:"before"`
	After  *uint64 `json:"after"`
}

type proofReport struct {
	Version     int          `json:"version"`
	Scope       string       `json:"scope"`
	Checks      []proofCheck `json:"checks"`
	Limitations []string     `json:"limitations"`
}

func runProof(system System, stdout, stderr io.Writer) int {
	if _, err := io.WriteString(stdout, "owned-smoke: proof-started\n"); err != nil {
		return 1
	}
	if _, err := io.WriteString(stderr, "owned-smoke: owned stderr\n"); err != nil {
		return 1
	}
	var effects ProofObservations
	if source, ok := system.(ProofSystem); ok {
		effects = source.ObserveProof()
	}
	report := proofReport{Version: 1, Scope: "owned-smoke-v1", Limitations: []string{
		"Owned fixture observations only; no absolute escape or security guarantee.",
		"Network checks send no packets; credential checks cover named keys and harmless sentinels, not all host credential locations.",
		"Timeout, cancellation, combined output limits and container cleanup require independent runner evidence.",
	}}
	for _, item := range []struct{ name, detail string }{
		{"nonroot", "uid65534"}, {"capabilities", "all capability sets zero"},
		{"no_new_privileges", "NoNewPrivs enabled"}, {"seccomp", "filter mode observed"},
		{"root_readonly", "owned marker open rejected with EROFS"},
		{"scratch_write_cleanup", "exclusive owned scratch written and removed"},
	} {
		report.Checks = append(report.Checks, proofCheck{Name: item.name, State: "OBSERVED", Detail: item.detail})
	}
	complete := true
	for _, item := range []struct {
		name, detail string
		observed     bool
	}{
		{"scratch_limit", "owned bounded write reached ENOSPC and cleanup completed", effects.ScratchLimit},
		{"network_unreachable", "no-send route lookup and local network checks agreed", effects.NetworkUnreachable},
		{"host_credentials_absent", "named credential keys and harmless host sentinels absent", effects.HostCredentialsAbsent},
	} {
		c := proofCheck{Name: item.name, State: "UNVERIFIABLE", Detail: "required bounded observation unavailable"}
		if item.observed {
			c.State = "OBSERVED"
			c.Detail = item.detail
		} else {
			complete = false
		}
		report.Checks = append(report.Checks, c)
	}
	for _, item := range []struct {
		name, detail string
		counter      CounterObservation
	}{
		{"cpu_limit", "bounded demand increased cpu.stat throttling counter", effects.CPU},
		{"memory_limit", "bounded owned child wait and memory.events limit effect", effects.Memory},
		{"pids_limit", "bounded owned child wait and pids.events limit effect", effects.Pids},
	} {
		c := proofCheck{Name: item.name, State: "UNVERIFIABLE", Detail: "required bounded observation unavailable"}
		if item.counter.Supported && item.counter.After > item.counter.Before {
			c.State = "OBSERVED"
			c.Detail = item.detail
			before, after := item.counter.Before, item.counter.After
			c.Before, c.After = &before, &after
		} else {
			complete = false
		}
		report.Checks = append(report.Checks, c)
	}
	// Fixed-size schema and literals: output cannot grow with system input.
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		return 1
	}
	if !complete {
		io.WriteString(stderr, "owned-smoke: unavailable\n")
		return 1
	}
	return 0
}
