//go:build !legacy

package main

import (
	"fmt"
	"time"
)

// MatrixKong runs protocol validation matrix tests
type MatrixKong struct {
	Scenarios  []string `kong:"flag,name='scenario',short='s',help='Test scenarios'"`
	Sources    []string `kong:"flag,name='source',help='Source protocols'"`
	Targets    []string `kong:"flag,name='target',help='Target protocols'"`
	Streaming  bool     `kong:"flag,name='streaming',help='Run only streaming tests'"`
	NonStream  bool     `kong:"flag,name='non-streaming',help='Run only non-streaming tests'"`
	JsonOutput bool     `kong:"flag,name='json',help='JSON output'"`
	Verbose    []bool   `kong:"flag,name='verbose',short='v',help='Verbose level'"`
	RecordDir  string   `kong:"flag,name='record-dir',help='Recording directory'"`
	ServerMode string   `kong:"flag,name='server-mode',help='Server reuse mode'"`
	BatchCount int      `kong:"flag,name='batch',help='Batch count'"`
}

func (m *MatrixKong) Run() error {
	cmd := newMatrixCommand()
	args := buildArgsFromMatrix(m)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func buildArgsFromMatrix(m *MatrixKong) []string {
	args := []string{}
	for _, s := range m.Scenarios {
		args = append(args, "--scenario", s)
	}
	for _, s := range m.Sources {
		args = append(args, "--source", s)
	}
	for _, t := range m.Targets {
		args = append(args, "--target", t)
	}
	if m.Streaming {
		args = append(args, "--streaming")
	}
	if m.NonStream {
		args = append(args, "--non-streaming")
	}
	if m.JsonOutput {
		args = append(args, "--json")
	}
	for i := 0; i < len(m.Verbose); i++ {
		args = append(args, "-v")
	}
	if m.RecordDir != "" {
		args = append(args, "--record-dir", m.RecordDir)
	}
	if m.ServerMode != "" {
		args = append(args, "--server-mode", m.ServerMode)
	}
	if m.BatchCount > 0 {
		args = append(args, "--batch", fmt.Sprintf("%d", m.BatchCount))
	}
	return args
}

// AgentKong runs agent e2e tests
type AgentKong struct {
	Mock      bool          `kong:"flag,name='mock',help='Use virtual upstream provider'"`
	Config    string        `kong:"flag,name='config',help='Config file for real providers'"`
	Prompt    string        `kong:"flag,name='prompt',help='Prompt to send (overrides positional and default)'"`
	Summary   string        `kong:"flag,name='summary',default='harness-summary.csv',help='Path to CSV summary file'"`
	OutputDir string        `kong:"flag,name='output-dir',help='Directory for full output files (default: harness-output/)'"`
	Resume    string        `kong:"flag,name='resume',help='Resume from previous run: skip recorded (agent,entry) pairs'"`
	Filter    []string      `kong:"flag,name='filter',help='Only run entries whose name matches (real-provider mode)'"`
	Timeout   time.Duration `kong:"flag,name='timeout',short='t',default='2m',help='Per-entry timeout (e.g. 30s, 2m). 0 disables.'"`
	AgentType string        `kong:"arg,optional,help='Agent type (claude, codex, opencode, batch)'"`
	Args      []string      `kong:"arg,optional,help='Optional prompt as positional args'"`
}

func (a *AgentKong) Run() error {
	cmd := newAgentCommand()
	args := []string{}
	if a.AgentType != "" {
		args = append(args, a.AgentType)
	}
	args = append(args, a.Args...)
	if a.Mock {
		args = append(args, "--mock")
	}
	if a.Config != "" {
		args = append(args, "--config", a.Config)
	}
	if a.Prompt != "" {
		args = append(args, "--prompt", a.Prompt)
	}
	if a.Summary != "" {
		args = append(args, "--summary", a.Summary)
	}
	if a.OutputDir != "" {
		args = append(args, "--output-dir", a.OutputDir)
	}
	if a.Resume != "" {
		args = append(args, "--resume", a.Resume)
	}
	for _, f := range a.Filter {
		args = append(args, "--filter", f)
	}
	args = append(args, "--timeout", a.Timeout.String())
	cmd.SetArgs(args)
	return cmd.Execute()
}

// ProviderKong runs real provider API tests
type ProviderKong struct {
	Test ProviderTestKong `kong:"cmd,help='Run provider tests'"`
	List ProviderListKong `kong:"cmd,help='List providers'"`
}

func (p *ProviderKong) Run() error {
	return fmt.Errorf("provider tests not yet implemented - see Phase 3 in specification")
}

// ProviderTestKong runs provider tests
type ProviderTestKong struct {
	Provider  string   `kong:"arg,optional,help='Provider name or UUID'"`
	Scenarios []string `kong:"flag,name='scenario',help='Test scenarios'"`
}

func (p *ProviderTestKong) Run() error {
	cmd := newProviderCommand()
	args := []string{"test"}
	if p.Provider != "" {
		args = append(args, p.Provider)
	}
	for _, s := range p.Scenarios {
		args = append(args, "--scenario", s)
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// ProviderListKong lists providers
type ProviderListKong struct{}

func (p *ProviderListKong) Run() error {
	cmd := newProviderCommand()
	cmd.SetArgs([]string{"list"})
	return cmd.Execute()
}

// InitConfigKong creates config file template
type InitConfigKong struct {
	Output string `kong:"flag,name='output',short='o',help='Output file path'"`
}

func (i *InitConfigKong) Run() error {
	return runInitConfig(i.Output)
}

// VersionKong shows version
type VersionKong struct{}

func (v *VersionKong) Run() error {
	fmt.Printf("Tingly-Box Protocol Validation Harness\n")
	fmt.Printf("Version:   %s\n", version)
	fmt.Printf("Commit:    %s\n", gitCommit)
	fmt.Printf("Built:     %s\n", buildTime)
	return nil
}
