# KubeSystemd

**kube-systemd** is a controller to help manage systemd services on each Node in clusters.

With clusters like minikube on hyberkit, which boot always from an ISO, 
**kube-systemd** could save configurations of systemd services and apply them after nodes started.

**kube-systemd** introduces CRD Unit to save all configurations.
Users can also set a job instead. The job will restart just after each node boot. 

```yaml
type Unit struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   UnitSpec   `json:"spec,omitempty"`
    Status UnitStatus `json:"status,omitempty"`
}

type UnitSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Specify an existed job which will restart once node boots up.
	// +optional
	Job corev1.ObjectReference `json:"job,omitempty"`

	// Defines a systemd unit.
	// +optional
	HostUnit HostSystemdUnit `json:"unit,omitempty"`
}

type HostSystemdUnit struct {
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
  unit:
    path: "/lib/systemd/system/systemd-timesyncd.service"
    config:
      "/etc/systemd/timesyncd.conf": |
        [Time]
        NTP=ntp1.aliyun.com
---
apiVersion: core.systemd.warmmetal.tech/v1
kind: Unit
metadata:
  name: binfmt-register
spec:
  job:
    kind: Job
    name: binfmt-register
    namespace: default
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
      {"apiVersion":"core.systemd.warmmetal.tech/v1","kind":"Unit","metadata":{"annotations":{},"name":"systemd-timesyncd.service"},"spec":{"unit":{"config":{"/etc/systemd/timesyncd.conf":"[Time]\nNTP=ntp1.aliyun.com\n"},"path":"/lib/systemd/system/systemd-timesyncd.service"}}}
  creationTimestamp: "2021-06-05T14:20:31Z"
  generation: 3
  name: systemd-timesyncd.service
  resourceVersion: "1179328"
  uid: f150a842-9804-4313-8f72-99ad3151cf46
spec:
  unit:
    config:
      /etc/systemd/timesyncd.conf: |
        [Time]
        NTP=ntp1.aliyun.com
    path: /lib/systemd/system/systemd-timesyncd.service
status:
  execTimestamp: "2021-06-05T14:23:43Z"
```
