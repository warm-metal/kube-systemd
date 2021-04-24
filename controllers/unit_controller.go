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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		err := startUnit(ctx, unit)
		if err != nil {
			unit.Status.Error = err.Error()
		}

		if err := r.Status().Update(ctx, unit); err != nil {
			return ctrl.Result{}, err
		}

		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func startUnit(ctx context.Context, unit *corev1.Unit) error {
	if len(unit.Spec.Path) == 0 {
		return xerrors.New("Spec.Path is required")
	}

	if !strings.HasPrefix(unit.Spec.Path, libSystemdDir) && !strings.HasPrefix(unit.Spec.Path, etcSystemdDir) {
		return xerrors.Errorf("Spec.Path must be in directory %q or %q", libSystemdDir, etcSystemdDir)
	}

	if len(unit.Spec.Definition) > 0 {
		if err := ioutil.WriteFile(unit.Spec.Path, []byte(unit.Spec.Definition), 0644); err != nil {
			return xerrors.Errorf("unable to write unit file %q: %s", unit.Spec.Path, err)
		}
	}

	for path, content := range unit.Spec.Config {
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

	systemctl := exec.CommandContext(ctx, "systemctl", "restart", filepath.Base(unit.Spec.Path))
	if err := systemctl.Start(); err != nil {
		return xerrors.Errorf("unable to restart service %q", filepath.Base(unit.Spec.Path))
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnitReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Unit{}).
		Complete(r)
}
