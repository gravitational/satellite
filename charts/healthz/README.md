# Healthz Helm Chart

Jenkins master and slave cluster utilizing the Jenkins Kubernetes plugin

## Chart Details

This chart will do the following:

* 1 x Healthz monitoring daemon with configured port exposed on an external LoadBalancer

## Installing the Chart

To install the chart with the release name `my-release`:

```bash
$ helm install --name my-release charts/healthz
```

## Configuration

The following tables lists the configurable parameters of the Healthz chart and their default values.

| Parameter                     | Description                                               | Default                                       |
| ----------------------------- | --------------------------------------------------------- | --------------------------------------------- |
| `healthz.name`                | Base chart resources name                                 | `healthz`                                     |
| `healthz.accesskey`           | Access key to fetch status from healthz                   | `akey`                                            |
| `healthz.debug`               | Enable/disable debug log level                            | `true`                                        |
| `healthz.checkinterval`       | K8S and ETCD services check interval (Go duration format) | `1m`                                          |
| `healthz.kube.addr`           | K8S API endpoint                                          | `http://localhost:8080`                       |
| `healthz.kube.nodesThreshold` | Lower limit of K8S nodes available                        | `75`                                          |
| `healthz.image.repo`          | Image repo                                                | `quay.io/gravitational/satellite`             |
| `healthz.image.tag`           | Image tag                                                 | `stable`                                      |
| `healthz.image.pullPolicy`    | Image pull policy                                         | `IfNotPresent`                                |
| `healthz.port`                | External service port                                     | `8080`                                        |
| `healthz.ssl.enabled`         | Enable/disable SSL on service port                        | `false`                                       |
| `healthz.ssl.cert`            | External service SSL certificate                          | ``                                            |
| `healthz.ssl.key`             | External service SSL key                                  | ``                                            |
| `healthz.ssl.ca`              | External service SSL CA                                   | ``                                            |
| `healthz.etcd.peers`          | Comma-separated ETCD service endpoints to check           | `http://localhost:4001,http://localhost:2380` |
| `healthz.etcd.cert`           | ETCD service SSL certificate                              | ``                                            |
| `healthz.etcd.key`            | ETCD service SSL key                                      | ``                                            |
| `healthz.etcd.ca`             | ETCD service SSL CA                                       | ``                                            |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart. For example,

```bash
$ helm install --name my-release -f values.yaml charts/healthz
```

> **Tip**: You can use the default [values.yaml](values.yaml)

