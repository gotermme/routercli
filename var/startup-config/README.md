# var/startup-config

This directory holds the saved startup-config file, the same "copy
running-config startup-config" and "erase startup-config" text file
Cisco and HP call startup-config, written and removed by su-config's
own commands in cmd/product/cmd_startup_config.go, and read back in
automatically at every process start by main.go's own
loadStartupConfig. See cmd/core/cmd_su_config.go for the reasoning
behind why that automatic replay needs no password of its own, and
etc/README.md's StartupConfigFile section for how a deployment points
routercli.yaml at a file, and a directory, other than this one.

This directory ships empty on a fresh checkout, matching a fresh
device that has never had "copy running-config startup-config" run
against it. The actual saved file, StartupConfigFile's own filename,
"startup-config" by default, is created the first time that command
runs, and is not meant to be checked into source control alongside
this README, since it holds a live deployment's own saved state, not
anything this framework itself ships.

This README exists so the directory itself, empty otherwise, survives
a checkout the same way var/log's own log files, and var/lang's and
var/tree's own real content, already keep those directories from
disappearing. Git does not track empty directories on its own.
