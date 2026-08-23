// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package config implements loading and validating routercli's system
configuration file. A project built on routercli needs one place that
says where its tree files live, whether a login is required, and how
strict its rate limiting is, without hardcoding any of that in main.go.
This package is that one place.

The configuration file is YAML, decoded with strict, unknown field
checking, so a typo in a property name is a startup error rather than a
silently ignored setting. A missing or empty configuration file is not
an error. LoadSystemConfig returns DefaultSystemConfig instead, so a
project can start running before it has written a configuration file of
its own. See LoadSystemConfig and SystemConfig for the full set of
settings and how each one is used.
*/
package config
