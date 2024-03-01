# NSO event subscriber

Subscribes to available Network Services Orchestrator event streams, optionally sending one
or more webhooks for each event to other services (such as Jenkins). Can be configured by CLI
arguments and/or a YAML-based configuration file (currently hardwired to ```nsoeventConfig.yaml```).
Use of the configuration file is required for the webhook definitions.

```commandline
❯ ./nsoevent --help
Usage:
  nsoevent [flags]
  nsoevent [command]

Available Commands:
  help        Help about any command
  info        show server info
  list        list available event streams
  subscribe   subscribe to one or more event streams

Flags:
  -d, --debug              enable debug output
  -h, --help               help for nsoevent
      --nocolor            disable colorized output
  -p, --password string    password for NSO API (default "admin")
      --pprofPort int      listen port for pprof server
  -t, --timeout duration   API timeout (default 3s)
      --url string         NSO API URL (http://IP:PORT)
  -u, --user string        user for NSO API (default "admin")
  -v, --verbose            enable verbose logging
      --version            version for nsoevent

Use "nsoevent [command] --help" for more information about a command.
```

One or more webhooks can be defined for each stream (by name) and each webhook supports multiple
filters per event. If using filters, ALL of the conditions must be met for the webhook to fire.

An example of the configuration file:
```yaml
---
nso:
  restconfAPI:      http://10.1.1.1:8080
  user:             admin
  password:         admin
  connectTimeout:   3s # seconds
  readTimeout:      10m # minutes. some NSO requests can take a long time

webhooks:
  - stream:         ncs-events
    url:            http://192.168.1.108:18080/generic-webhook-trigger/invoke
    token:          Test-Pipeline
    filter:
      node:
        - name:     state
          value:    deleted
  - stream:         NETCONF
    disable:        false
    url:            http://192.168.1.108:18080/generic-webhook-trigger/invoke
    token:          NETGITOPS-Pipeline
    filter:
      event:        netconf-config-change
      node:
        - name:     datastore
          value:    running
        - name:     target
          value:    .*ncs:name='CT_RTR_McCampbell'.*
```

## Webhooks
The webhooks contain information about the triggering event with some high-level details extracted
from the original XML event structure (which is included). The high-level details in JSON are more
easily consumed by Jenkins' generic webhook plugin or something like ```jq```.

An example from an event on the NETCONF event stream:
```json
{
  "source": "172.16.1.1:48888",
  "stream": "NETCONF",
  "eventname": "netconf-config-change",
  "user": "admin",
  "host": "127.0.0.1",
  "datastore": "running",
  "devices": [
    "R0",
    "R1"
  ],
  "edits": {
    "R0": [
      {
        "target": "/ncs:devices/ncs:device[ncs:name='R0']/ncs:config/ios:banner/ios:motd",
        "operation": "replace"
      }
    ],
    "R1": [
      {
        "target": "/ncs:devices/ncs:device[ncs:name='R1']/ncs:config/ios:banner/ios:motd",
        "operation": "replace"
      }
    ]
  },
  "event": "<nsoevent><eventTime>2021-01-26T18:27:43.994194+00:00</eventTime><netconf-config-change xmlns='urn:ietf:params:xml:ns:yang:ietf-netconf-notifications'>  <changed-by>    <username>admin</username>    <session-id>0</session-id>    <source-host>127.0.0.1</source-host>  </changed-by>  <datastore>running</datastore>  <edit>    <target xmlns:ios=\"urn:ios\" xmlns:ncs=\"http://tail-f.com/ns/ncs\">/ncs:devices/ncs:device[ncs:name='R0']/ncs:config/ios:banner/ios:motd</target>    <operation>replace</operation>  </edit>  <edit>    <target xmlns:ios=\"urn:ios\" xmlns:ncs=\"http://tail-f.com/ns/ncs\">/ncs:devices/ncs:device[ncs:name='R1']/ncs:config/ios:banner/ios:motd</target>    <operation>replace</operation>  </edit></netconf-config-change></nsoevent>"
}
```
