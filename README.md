# KubeSystemd

**kube-systemd** is a controller to help manage systemd services on each Node in clusters.

With clusters like minikube on hyberkit, which boot always from an ISO, 
**kube-systemd** could save configurations of systemd services and apply them after nodes started.

**kube-systemd** introduces CRD Unit to save all configurations.

```yaml
type Unit struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   UnitSpec   `json:"spec,omitempty"`
    Status UnitStatus `json:"status,omitempty"`
}

type UnitSpec struct {
    // Path defines the absolute path on the host of the unit.
    Path string `json:"path,omitempty"`

    // Definition specifies the unit definition. If set, it is written to the unit configuration which Path defines.
    // Or, the original unit on the host will be used.
    // +optional
    Definition string `json:"definition,omitempty"`

    // Config specifies config files and contents on the host with respect to the systemd unit.
    // The key is the absolute path of the configuration file. And, the value is the file content.
    // +optional
    Config map[string]string `json:"config,omitempty"`
}
```

## Install
```shell script
kubectl apply -f https://raw.githubusercontent.com/warm-metal/kube-systemd/master/config/samples/install.yaml
```

## Demo

We can create a unit to modify NTP server configuration in a minikube cluster to make sure the cluster clock is always
synchronized to the NTP server.

```yaml
apiVersion: core.systemd.warmmetal.tech/v1
kind: Unit
metadata:
  name: systemd-timesyncd.service
spec:
  path: "/lib/systemd/system/systemd-timesyncd.service"
  config:
    "/etc/systemd/timesyncd.conf": |
      [Time]
      NTP=ntp1.aliyun.com
```

After the unit executed, we could see that its status changed.
That is, `status.execTimestamp` is updated to the time last executed.
If errors raised, the `status.error` would be also updated. 

```yaml
apiVersion: core.systemd.warmmetal.tech/v1
kind: Unit
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"core.systemd.warmmetal.tech/v1","kind":"Unit","metadata":{"annotations":{},"name":"systemd-timesyncd.service"},"spec":{"config":{"/etc/systemd/timesyncd.conf":"[Time]\nNTP=ntp1.aliyun.com\n"},"path":"/lib/systemd/system/systemd-timesyncd.service"}}
  creationTimestamp: "2021-03-10T08:52:30Z"
  generation: 1
  managedFields:
  - apiVersion: core.systemd.warmmetal.tech/v1
    fieldsType: FieldsV1
    fieldsV1:
      f:metadata:
        f:annotations:
          .: {}
          f:kubectl.kubernetes.io/last-applied-configuration: {}
      f:spec:
        .: {}
        f:config:
          .: {}
          f:/etc/systemd/timesyncd.conf: {}
        f:path: {}
    manager: kubectl-client-side-apply
    operation: Update
    time: "2021-03-10T08:52:30Z"
  - apiVersion: core.systemd.warmmetal.tech/v1
    fieldsType: FieldsV1
    fieldsV1:
      f:status:
        .: {}
        f:execTimestamp: {}
    manager: manager
    operation: Update
    time: "2021-03-10T08:52:30Z"
  name: systemd-timesyncd.service
  resourceVersion: "208241"
  uid: ad1d4311-b26b-4261-8551-f81f659fa2d3
spec:
  config:
    /etc/systemd/timesyncd.conf: |
      [Time]
      NTP=ntp1.aliyun.com
  path: /lib/systemd/system/systemd-timesyncd.service
status:
  execTimestamp: "2021-03-10T09:08:46Z"
```
