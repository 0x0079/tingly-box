//go:build !legacy

package command

import (
	"bufio"
	"os"
)

// ProviderCmdKong is the Kong version of provider command. The hidden
// Interactive subcommand is marked default so `tingly-box provider` (no
// further args) drops into interactive management, matching legacy behavior.
type ProviderCmdKong struct {
	Interactive ProviderInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive provider management'"`
	Add         ProviderAddCmdKong         `kong:"cmd,help='Add a new provider'"`
	List        ProviderListCmdKong        `kong:"cmd,help='List all providers'"`
	Delete      ProviderDeleteCmdKong      `kong:"cmd,help='Delete a provider (interactive)'"`
	Update      ProviderUpdateCmdKong      `kong:"cmd,help='Update a provider (interactive)'"`
	Get         ProviderGetCmdKong         `kong:"cmd,help='Get provider details by name'"`
}

// ProviderInteractiveCmdKong runs the interactive provider menu.
type ProviderInteractiveCmdKong struct{}

func (p *ProviderInteractiveCmdKong) Run(appManager *AppManager) error {
	return runProviderInteractiveMode(appManager)
}

// ProviderAddCmdKong adds a new provider
type ProviderAddCmdKong struct {
	Name     string `kong:"arg,optional,help='Provider name'"`
	BaseURL  string `kong:"arg,optional,help='API base URL'"`
	Token    string `kong:"arg,optional,help='API token'"`
	APIStyle string `kong:"arg,optional,help='API style (openai, anthropic)'"`
}

func (p *ProviderAddCmdKong) Run(appManager *AppManager) error {
	args := []string{}
	if p.Name != "" {
		args = append(args, p.Name)
	}
	if p.BaseURL != "" {
		args = append(args, p.BaseURL)
	}
	if p.Token != "" {
		args = append(args, p.Token)
	}
	if p.APIStyle != "" {
		args = append(args, p.APIStyle)
	}
	return runAdd(appManager, args)
}

// ProviderListCmdKong lists all providers
type ProviderListCmdKong struct{}

func (p *ProviderListCmdKong) Run(appManager *AppManager) error {
	return runProviderList(appManager)
}

// ProviderDeleteCmdKong deletes a provider in interactive mode.
type ProviderDeleteCmdKong struct{}

func (p *ProviderDeleteCmdKong) Run(appManager *AppManager) error {
	return runProviderDeleteInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ProviderUpdateCmdKong updates a provider in interactive mode.
type ProviderUpdateCmdKong struct{}

func (p *ProviderUpdateCmdKong) Run(appManager *AppManager) error {
	return runProviderUpdateInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ProviderGetCmdKong displays a provider's details. Without a name it drops
// into interactive selection.
type ProviderGetCmdKong struct {
	Name string `kong:"arg,optional,help='Provider name'"`
}

func (p *ProviderGetCmdKong) Run(appManager *AppManager) error {
	if p.Name == "" {
		return runProviderGetInteractive(appManager, bufio.NewReader(os.Stdin))
	}
	return runProviderGet(appManager, p.Name)
}
