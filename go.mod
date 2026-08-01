module github.com/lanceman/zqk

go 1.26

toolchain go1.26.0

require (
	github.com/briandowns/spinner v1.23.2
	github.com/fatih/color v1.18.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/go-cmp v0.7.0
	github.com/jedib0t/go-pretty/v6 v6.8.0
	github.com/joho/godotenv v1.5.1
	github.com/mitchellh/go-ps v1.0.0
	github.com/neo4j/neo4j-go-driver/v5 v5.28.4
	github.com/robfig/cron/v3 v3.0.1
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.46.0
	golang.org/x/sync v0.20.0
	golang.org/x/sys v0.46.0
	golang.org/x/term v0.44.0
	golang.org/x/tools v0.45.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)

replace github.com/lanceman/zqk/pkg/agentdelivery => ./pkg/agentdelivery

replace github.com/lanceman/zqk/pkg/agentprompt => ./pkg/agentprompt

replace github.com/lanceman/zqk/pkg/agentrules => ./pkg/agentrules

replace github.com/lanceman/zqk/pkg/aliases => ./pkg/aliases

replace github.com/lanceman/zqk/pkg/appledouble => ./pkg/appledouble

replace github.com/lanceman/zqk/pkg/brand => ./pkg/brand

replace github.com/lanceman/zqk/pkg/cleanup => ./pkg/cleanup

replace github.com/lanceman/zqk/pkg/cli => ./pkg/cli

replace github.com/lanceman/zqk/pkg/cliexamples => ./pkg/cliexamples

replace github.com/lanceman/zqk/pkg/clihooks => ./pkg/clihooks

replace github.com/lanceman/zqk/pkg/concurrency => ./pkg/concurrency

replace github.com/lanceman/zqk/pkg/config => ./pkg/config

replace github.com/lanceman/zqk/pkg/context => ./pkg/context

replace github.com/lanceman/zqk/pkg/contextevents => ./pkg/contextevents

replace github.com/lanceman/zqk/pkg/convergerollup => ./pkg/convergerollup

replace github.com/lanceman/zqk/pkg/coordination => ./pkg/coordination

replace github.com/lanceman/zqk/pkg/cursorrules => ./pkg/cursorrules

replace github.com/lanceman/zqk/pkg/datacell => ./pkg/datacell

replace github.com/lanceman/zqk/pkg/datacellregistry => ./pkg/datacellregistry

replace github.com/lanceman/zqk/pkg/diagnostics => ./pkg/diagnostics

replace github.com/lanceman/zqk/pkg/dispatch => ./pkg/dispatch

replace github.com/lanceman/zqk/pkg/docman => ./pkg/docman

replace github.com/lanceman/zqk/pkg/domain => ./pkg/domain

replace github.com/lanceman/zqk/pkg/drifthotspots => ./pkg/drifthotspots

replace github.com/lanceman/zqk/pkg/errfmt => ./pkg/errfmt

replace github.com/lanceman/zqk/pkg/featureflags => ./pkg/featureflags

replace github.com/lanceman/zqk/pkg/functional => ./pkg/functional

replace github.com/lanceman/zqk/pkg/gantt => ./pkg/gantt

replace github.com/lanceman/zqk/pkg/git => ./pkg/git

replace github.com/lanceman/zqk/pkg/goroutinelabels => ./pkg/goroutinelabels

replace github.com/lanceman/zqk/pkg/gotestparse => ./pkg/gotestparse

replace github.com/lanceman/zqk/pkg/graph => ./pkg/graph

replace github.com/lanceman/zqk/pkg/healthcheck => ./pkg/healthcheck

replace github.com/lanceman/zqk/pkg/httpheaders => ./pkg/httpheaders

replace github.com/lanceman/zqk/pkg/interactive => ./pkg/interactive

replace github.com/lanceman/zqk/pkg/kindnames => ./pkg/kindnames

replace github.com/lanceman/zqk/pkg/kindsynonyms => ./pkg/kindsynonyms

replace github.com/lanceman/zqk/pkg/lifecycle => ./pkg/lifecycle

replace github.com/lanceman/zqk/pkg/loader => ./pkg/loader

replace github.com/lanceman/zqk/pkg/logging => ./pkg/logging

replace github.com/lanceman/zqk/pkg/mcp => ./pkg/mcp

replace github.com/lanceman/zqk/pkg/metrics => ./pkg/metrics

replace github.com/lanceman/zqk/pkg/metricsrecording => ./pkg/metricsrecording

replace github.com/lanceman/zqk/pkg/migration => ./pkg/migration

replace github.com/lanceman/zqk/pkg/nildecode => ./pkg/nildecode

replace github.com/lanceman/zqk/pkg/objectget => ./pkg/objectget

replace github.com/lanceman/zqk/pkg/objects => ./pkg/objects

replace github.com/lanceman/zqk/pkg/observability => ./pkg/observability

replace github.com/lanceman/zqk/pkg/observer => ./pkg/observer

replace github.com/lanceman/zqk/pkg/operational => ./pkg/operational

replace github.com/lanceman/zqk/pkg/outputtypes => ./pkg/outputtypes

replace github.com/lanceman/zqk/pkg/paths => ./pkg/paths

replace github.com/lanceman/zqk/pkg/pipeline => ./pkg/pipeline

replace github.com/lanceman/zqk/pkg/policyinterrupt => ./pkg/policyinterrupt

replace github.com/lanceman/zqk/pkg/precommit => ./pkg/precommit

replace github.com/lanceman/zqk/pkg/process => ./pkg/process

replace github.com/lanceman/zqk/pkg/processhygiene => ./pkg/processhygiene

replace github.com/lanceman/zqk/pkg/processing => ./pkg/processing

replace github.com/lanceman/zqk/pkg/projecttemp => ./pkg/projecttemp

replace github.com/lanceman/zqk/pkg/quality => ./pkg/quality

replace github.com/lanceman/zqk/pkg/quick => ./pkg/quick

replace github.com/lanceman/zqk/pkg/rollback => ./pkg/rollback

replace github.com/lanceman/zqk/pkg/runtime => ./pkg/runtime

replace github.com/lanceman/zqk/pkg/safepath => ./pkg/safepath

replace github.com/lanceman/zqk/pkg/scenario => ./pkg/scenario

replace github.com/lanceman/zqk/pkg/scheduler => ./pkg/scheduler

replace github.com/lanceman/zqk/pkg/specbuilder => ./pkg/specbuilder

replace github.com/lanceman/zqk/pkg/specorigination => ./pkg/specorigination

replace github.com/lanceman/zqk/pkg/stewardbase => ./pkg/stewardbase

replace github.com/lanceman/zqk/pkg/storage => ./pkg/storage

replace github.com/lanceman/zqk/pkg/storagetesting => ./pkg/storagetesting

replace github.com/lanceman/zqk/pkg/strutil => ./pkg/strutil

replace github.com/lanceman/zqk/pkg/syscallutil => ./pkg/syscallutil

replace github.com/lanceman/zqk/pkg/termfd => ./pkg/termfd

replace github.com/lanceman/zqk/pkg/testenvroot => ./pkg/testenvroot

replace github.com/lanceman/zqk/pkg/testing => ./pkg/testing

replace github.com/lanceman/zqk/pkg/testjobgen => ./pkg/testjobgen

replace github.com/lanceman/zqk/pkg/testkit => ./pkg/testkit

replace github.com/lanceman/zqk/pkg/testpackageconcurrency => ./pkg/testpackageconcurrency

replace github.com/lanceman/zqk/pkg/testscan => ./pkg/testscan

replace github.com/lanceman/zqk/pkg/testservices => ./pkg/testservices

replace github.com/lanceman/zqk/pkg/testtiming => ./pkg/testtiming

replace github.com/lanceman/zqk/pkg/translation => ./pkg/translation

replace github.com/lanceman/zqk/pkg/tray => ./pkg/tray

replace github.com/lanceman/zqk/pkg/validation => ./pkg/validation

replace github.com/lanceman/zqk/pkg/walutil => ./pkg/walutil

replace github.com/lanceman/zqk/pkg/when => ./pkg/when

replace github.com/lanceman/zqk/pkg/zqkenv => ./pkg/zqkenv

replace github.com/lanceman/zqk/pkg/zqktime => ./pkg/zqktime
