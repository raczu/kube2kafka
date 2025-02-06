# kube2kafka

[![development][development badge]][development workflow]
[![go Report Card][go report badge]][go report]
[![coverage][coverage badge]][sonarqube]

kube2kafka is a simple tool for exporting k8s events to Kafka with support for filtering them using
defined filters based on regular expressions and customizing their final structure using selectors
based on [text/template][text template] syntax.

## Filtering observed events

Filters allow to control the flow of events, meaning they define which events should be exported to
Kafka. If observed event matches all criteria of at least one filter, it will be exported. All
fields in the [filter][filter] support regular expressions, which allows for a high level of
flexibility in defining the criteria &ndash; it is not required to define all possible fields within
single filter. Below is an example filter definition in the configuration file:

```yaml
filters:
  - kind: "Pod"
    namespace: "default|kube-system"
    reason: "(?i)created"
    message: ".*"
    type: "Normal|Warning"
    component: "^kubelet$"
```

## Customizing the events payload

Selectors allow to customize the final structure of the exported events. They are defined using the
two fields: `key` and `value`. The `key` field is a string that acts as the identifier of the event
value in the final structure, while the `value` follows the [text/template][text template] syntax
and is used to extract the value from the [event][event]. Below is an example selector definition in
the configuration file:

```yaml
selectors:
  - key: cluster
    value: "{{ .ClusterName }}"
  - key: kind
    value: "{{ .InvolvedObject.Kind }}"
  - key: namespace
    value: "{{ .Namespace }}"
```

## Configuration

The most important aspect of the configuration is defining the cluster name to identify the source
of exported events. Equally important is configuring the connection to Apache Kafka, including
brokers, topic, and authentication mechanisms if required. Additionally, selecting an appropriate
buffer size is crucial for handling expected traffic &ndash; if the traffic is high, older, not yet
processed or exported events may be overwritten when the buffer reaches its limit. A sample
configuration file with descriptions of all options can be found in the `config/config-example.yml`.

## Deployment

Deploying kube2kafka is simple and requires to create namespace, either `kube2kafka` or custom one
(but remember to update the `namespace` field in the manifests). All necessary manifests are located
in the `deploy` directory and should be applied in the following order:

```bash
kubectl apply -f deploy/rbac.yml
kubectl apply -f deploy/config.yml
kubectl apply -f deploy/deployment.yml
```

Before applying the configuration manifest, ensure to update the `config.yml` according to your
specific requirements, such as topic name, brokers, filters, and other relevant settings.
Additionally, in `deployment.yaml`, adjust the resource limits and requests based on your workload
needs.

## Troubleshooting

In case of any issues, the logs can be checked using the following command:

```bash
kubectl logs -n kube2kafka -f `kubectl get pods -A -l app=kube2kafka -o jsonpath='{.items[0].metadata.name}'`
```

If you suspect that some events are not exported, you can increase the verbosity of logger to debug
&ndash; it will log detailed information about the actions taken by filtering mechanism, helping you
identify issues. You can configure the logger using flags listed below, along with their available
arguments:

```text
-log-devel
    set logger to development mode
-log-encoder value
    log encoder (json or console)
-log-level value
    configure verbosity of logger (debug, info, warn, error)
-log-stacktrace-level value
    configure verbosity of stacktrace level (info, error, panic)
-log-time-encoder value
    time encoder (rfc3339, rfc3339nano, iso8601, millis, nanos)
```

Adjusting these settings will give you deeper insight into the kube2kafka behavior and help you
understand what's happening with the events.

[development workflow]: https://github.com/raczu/kube2kafka/actions/workflows/development.yml
[development badge]: https://github.com/raczu/kube2kafka/actions/workflows/development.yml/badge.svg?event=push
[go report]: https://goreportcard.com/report/github.com/raczu/kube2kafka
[go report badge]: https://goreportcard.com/badge/github.com/raczu/kube2kafka
[sonarqube]: https://sonarcloud.io/summary/new_code?id=raczu_kube2kafka
[coverage badge]: https://sonarcloud.io/api/project_badges/measure?project=raczu_kube2kafka&metric=coverage
[text template]: https://pkg.go.dev/text/template
[filter]: https://pkg.go.dev/github.com/raczu/kube2kafka/pkg/processor#Filter
[event]: https://pkg.go.dev/k8s.io/api/core/v1#Event
