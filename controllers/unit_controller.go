/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"golang.org/x/xerrors"
	"io/ioutil"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "github.com/warm-metal/kube-systemd/api/v1"
)

// UnitReconciler reconciles a Unit object
type UnitReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	SysUpTime time.Time
}

const (
	configurationDir = "/etc"
	libSystemdDir    = "/lib/systemd"
	etcSystemdDir    = "/etc/systemd"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch
//+kubebuilder:rbac:groups=core.systemd.warmmetal.tech,resources=units,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core.systemd.warmmetal.tech,resources=units/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core.systemd.warmmetal.tech,resources=units/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Unit object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *UnitReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("unit", req.NamespacedName)

	list := &corev1.UnitList{}
	if err := r.List(ctx, list); err != nil {
		return ctrl.Result{}, err
	}

	nextUnits := make([]*corev1.Unit, 0, len(list.Items))
	for i := range list.Items {
		unit := list.Items[i]
		if len(unit.Status.Error) > 0 || !unit.Status.ExecTimestamp.After(r.SysUpTime) {
			nextUnits = append(nextUnits, &list.Items[i])
		}
	}

	sort.Slice(nextUnits, func(i, j int) bool {
		return nextUnits[i].Name < nextUnits[j].Name
	})

	now := metav1.Now()
	for i := range nextUnits {
		unit := nextUnits[i]
		unit.Status.ExecTimestamp = now
		// It may lead to the container exit to restart some unit
		if err := r.Status().Update(ctx, unit); err != nil {
			return ctrl.Result{}, err
		}

		updatedUnit := corev1.Unit{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(unit), &updatedUnit); err != nil {
			return ctrl.Result{}, err
		}

		unit = &updatedUnit

		err := r.startUnit(ctx, unit)
		if err != nil {
			unit.Status.Error = err.Error()
		} else {
			unit.Status.Error = ""
		}

		if err := r.Status().Update(ctx, unit); err != nil {
			return ctrl.Result{}, err
		}

		if err != nil {
			if err == errRunning {
				err = nil
			}

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *UnitReconciler) startUnit(ctx context.Context, unit *corev1.Unit) error {
	if unit.Spec.Job.Namespace != "" && unit.Spec.Job.Name != "" {
		return r.restartJob(ctx, unit)
	} else if unit.Spec.HostUnit.Path != "" {
		return execHostUnit(ctx, &unit.Spec.HostUnit)
	} else {
		return xerrors.New("Job or HostUnit is required")
	}
}

var (
	enabled    = true
	errRunning = xerrors.New("Running")
)

func (r *UnitReconciler) restartJob(ctx context.Context, unit *corev1.Unit) error {
	job := batchv1.Job{}
	r.Log.Info("fetch job", "name", unit.Spec.Job.Name, "namespace", unit.Spec.Job.Namespace)
	err := r.Get(ctx, client.ObjectKey{Namespace: unit.Spec.Job.Namespace, Name: unit.Spec.Job.Name}, &job)
	if err != nil {
		return err
	}

	r.Log.Info("job status", "succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed, "completeAt", job.Status.CompletionTime)

	if job.Status.Succeeded > 0 && job.Status.Failed == 0 && job.Status.CompletionTime != nil && job.Status.CompletionTime.After(r.SysUpTime) {
		return nil
	}

	podName := job.Name + "-systemd"
	pod := v1.Pod{}
	if err = r.Get(ctx, client.ObjectKey{Namespace: unit.Spec.Job.Namespace, Name: podName}, &pod); err == nil {
		r.Log.Info("got pod", "pod", podName, "phase", pod.Status.Phase)
		if pod.CreationTimestamp.Time.Before(r.SysUpTime) ||
			(pod.Status.Phase == v1.PodFailed && pod.Status.StartTime != nil && time.Now().Sub(pod.Status.StartTime.Time) > time.Minute) {
			r.Log.Info("pod was created before node restarting. will delete and create a new one")
			if err = r.Delete(ctx, &pod); err != nil {
				return err
			}

			// force creating a new pod
			err = errors.NewNotFound(schema.GroupResource{
				Group:    pod.GroupVersionKind().Group,
				Resource: pod.Kind,
			}, pod.Name)
		}
	}

	if err != nil {
		r.Log.Info("create pod", "pod", podName, "fetchErr", err)
		pod = v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: job.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         unit.APIVersion,
						Kind:               unit.Kind,
						Name:               unit.Name,
						UID:                unit.UID,
						Controller:         &enabled,
						BlockOwnerDeletion: &enabled,
					},
				},
			},
			Spec: job.Spec.Template.Spec,
		}

		if err := r.Create(ctx, &pod); err != nil {
			return err
		}

		return errRunning
	}

	if pod.Status.Phase == v1.PodSucceeded {
		if err = r.Delete(ctx, &pod); err != nil {
			r.Log.Error(err, "unable to delete pod", "pod", pod.Name)
		}
		return nil
	}

	if pod.Status.Phase == v1.PodFailed {
		return xerrors.New(pod.Status.Reason)
	}

	return errRunning
}

func execHostUnit(ctx context.Context, unit *corev1.HostSystemdUnit) error {
	if !strings.HasPrefix(unit.Path, libSystemdDir) && !strings.HasPrefix(unit.Path, etcSystemdDir) {
		return xerrors.Errorf("Spec.Path must be in directory %q or %q", libSystemdDir, etcSystemdDir)
	}

	if len(unit.Definition) > 0 {
		if err := ioutil.WriteFile(unit.Path, []byte(unit.Definition), 0644); err != nil {
			return xerrors.Errorf("unable to write unit file %q: %s", unit.Path, err)
		}
	}

	for path, content := range unit.Config {
		if !strings.HasPrefix(path, configurationDir) {
			return xerrors.Errorf("config must be in directory %q", configurationDir)
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return xerrors.Errorf("unable to create dir %q: %s", path, err)
		}

		if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
			return xerrors.Errorf("unable to write config %q: %s", path, err)
		}
	}

	systemctl := exec.CommandContext(ctx, "systemctl", "restart", filepath.Base(unit.Path))
	if err := systemctl.Start(); err != nil {
		return xerrors.Errorf("unable to restart service %q", filepath.Base(unit.Path))
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnitReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Unit{}).
		Owns(&v1.Pod{}).
		Complete(r)
}
