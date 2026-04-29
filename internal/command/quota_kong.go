//go:build !legacy

package command

// QuotaCmdKong is the Kong version of quota command. The hidden Default
// subcommand is marked default so `tingly-box quota` (no further args)
// behaves like `quota list`, matching legacy behavior.
type QuotaCmdKong struct {
	Default QuotaDefaultCmdKong `kong:"cmd,name='default',default='1',hidden,help='List all provider quotas (default)'"`
	List    QuotaListCmdKong    `kong:"cmd,help='List all provider quotas'"`
	Get     QuotaGetCmdKong     `kong:"cmd,help='Get provider quota details'"`
	Refresh QuotaRefreshCmdKong `kong:"cmd,help='Refresh provider quota data'"`
	Summary QuotaSummaryCmdKong `kong:"cmd,help='Show quota summary'"`
}

// QuotaDefaultCmdKong is the no-subcommand default that runs the list view.
type QuotaDefaultCmdKong struct{}

func (q *QuotaDefaultCmdKong) Run(appManager *AppManager) error {
	return runQuotaList(appManager)
}

// QuotaListCmdKong lists quotas
type QuotaListCmdKong struct {
	Refresh bool `kong:"flag,name='refresh',short='r',help='Refresh before listing'"`
}

func (q *QuotaListCmdKong) Run(appManager *AppManager) error {
	if q.Refresh {
		return runQuotaRefresh(appManager)
	}
	return runQuotaList(appManager)
}

// QuotaGetCmdKong gets quota details
type QuotaGetCmdKong struct {
	Provider string `kong:"arg,optional,help='Provider name or UUID'"`
	Refresh  bool   `kong:"flag,name='refresh',short='r',help='Refresh before displaying'"`
}

func (q *QuotaGetCmdKong) Run(appManager *AppManager) error {
	if q.Provider == "" {
		return runQuotaGetInteractive(appManager)
	}
	return runQuotaGet(appManager, q.Provider, q.Refresh)
}

// QuotaRefreshCmdKong refreshes quota data
type QuotaRefreshCmdKong struct {
	Provider string `kong:"arg,optional,help='Provider name or UUID'"`
}

func (q *QuotaRefreshCmdKong) Run(appManager *AppManager) error {
	if q.Provider == "" {
		return runQuotaRefresh(appManager)
	}
	return runQuotaRefreshProvider(appManager, q.Provider)
}

// QuotaSummaryCmdKong shows quota summary
type QuotaSummaryCmdKong struct{}

func (q *QuotaSummaryCmdKong) Run(appManager *AppManager) error {
	return runQuotaSummary(appManager)
}
