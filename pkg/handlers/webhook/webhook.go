/*
Copyright 2018 Bitnami

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

package webhook

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bitnami-labs/kubewatch/config"
	"github.com/bitnami-labs/kubewatch/pkg/event"
	"github.com/bitnami-labs/kubewatch/pkg/utils"
	"github.com/sirupsen/logrus"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2 "k8s.io/api/autoscaling/v2"
	batch_v1 "k8s.io/api/batch/v1"
	api_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
)

const (
	RiskLevelMedium = "medium"
	RiskLevelHigh   = "high"
	RiskLevelLow    = "low"
)

var webhookErrMsg = `
%s

You need to set Webhook url, and Webhook cert if you use self signed certificates,
using "--url/-u" and "--cert", or using environment variables:

export KW_WEBHOOK_URL=webhook_url
export KW_WEBHOOK_CERT=/path/of/cert

Command line flags will override environment variables

`

// Webhook handler implements handler.Handler interface,
// Notify event to Webhook channel
type Webhook struct {
	Url string
}

// Custom webhook message and only will be enabled when the custom_webhook_output flag is set to true
type CustomWebhookMessage struct {
	Type        string            `json: type`
	Name        string            `json: name`
	Summary     string            `json : summary`
	Pod         string            `json: pod`
	Entity      string            `json: entity`
	Env         string            `json: env`
	ServiceName string            `json: service_name`
	Action      string            `json: action`
	CreatedAt   time.Time         `json: created_at`
	ActionBy    string            `json: action_by`
	RiskLevel   string            `json: risk_level`
	Metadata    map[string]string `json: metadata`
	Data        objectData        `json: data`
}

type WebhookMessage struct {
	EventMeta EventMeta `json:"eventmeta"`
	Text      string    `json:"text"`
	Time      time.Time `json:"time"`
}

// EventMeta containes the meta data about the event occurred
type EventMeta struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
}

// Init prepares Webhook configuration
func (m *Webhook) Init(c *config.Config) error {
	url := c.Handler.Webhook.Url
	cert := c.Handler.Webhook.Cert
	tlsSkip := c.Handler.Webhook.TlsSkip

	if url == "" {
		url = os.Getenv("KW_WEBHOOK_URL")
	}
	if cert == "" {
		cert = os.Getenv("KW_WEBHOOK_CERT")
	}

	m.Url = url

	if tlsSkip {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		if cert == "" {
			logrus.Printf("No webhook cert is given")
		} else {
			caCert, err := ioutil.ReadFile(cert)
			if err != nil {
				logrus.Printf("%s\n", err)
				return err
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: caCertPool}
		}

	}

	return checkMissingWebhookVars(m)
}

// Handle handles an event.
func (m *Webhook) Handle(e event.Event) {
	webhookMessage := prepareWebhookMessage(e, m)
	if objectGenerationChangeCheck(e) {
		err := postMessage(m.Url, webhookMessage)
		if err != nil {
			logrus.Printf("%s\n", err)
			return
		}

		logrus.Printf("Message successfully sent to %s at %s ", m.Url, time.Now())
	}
}

func objectGenerationChangeCheck(e event.Event) bool {
	object := utils.GetObjectMetaData(e.Obj)
	oldObject := utils.GetObjectMetaData(e.OldObj)
	if object.GetNamespace() == "kube-system" && e.Kind == "ConfigMap" {
		return false
	}
	if object.GetNamespace() == "gke-cluster-dataproc-pgv2-poc" && e.Kind == "ConfigMap" {
		return false
	}
	if object.GetGeneration() == 0 {
		return object.GetResourceVersion() != oldObject.GetResourceVersion()
	}
	return object.GetGeneration() != oldObject.GetGeneration()
}

func checkMissingWebhookVars(s *Webhook) error {
	if s.Url == "" {
		return fmt.Errorf(webhookErrMsg, "Missing Webhook url")
	}
	return nil
}

func prepareWebhookMessage(e event.Event, m *Webhook) *CustomWebhookMessage {

	kind := strings.ToLower(e.Kind)
	pod := extractLabels(e, "pod")
	entity := extractLabels(e, "entity")
	env := extractLabels(e, "environment")
	serviceName := extractLabels(e, "service")
	actionBy := getActionBy(kind)
	data := extractObjectDetails(e)

	return &CustomWebhookMessage{
		Type:        kind,
		Name:        e.Name,
		Summary:     e.Message(),
		Pod:         pod,
		Entity:      entity,
		Env:         env,
		ServiceName: serviceName,
		Action:      strings.ToLower(e.Reason),
		CreatedAt:   time.Now(),
		ActionBy:    actionBy,
		RiskLevel:   e.Status,
		Metadata:    map[string]string{},
		Data:        *data,
	}
}

type objectData struct {
	CurrentConfigName      string      `json: current_config_name`
	CurrentConfigNamespace string      `json: current_config_namespace`
	CurrentConfigSpec      interface{} `json: current_config_spec`
	OldConfigName          string      `json: old_config_name`
	OldConfigNamespace     string      `json: old_config_namespace`
	OldConfigSpec          interface{} `json: old_config_spec`
}

func extractObjectDetails(e event.Event) *objectData {
	var data objectData

	var obj interface{}
	var oldobj interface{}

	switch e.Kind {
	case "Deployment":
		var ok bool
		obj, ok = e.Obj.(*apps_v1.Deployment)
		oldobj, _ = e.OldObj.(*apps_v1.Deployment)

		if !ok {
			fmt.Println("Error casting object to Deployment type")
			return &data
		}
	case "Service":
		var ok bool
		obj, ok = e.Obj.(*api_v1.Service)
		oldobj, _ = e.OldObj.(*api_v1.Service)

		if !ok {
			fmt.Println("Error casting object to service type")
			return &data
		}
	case "Ingress":
		var ok bool
		obj, ok = e.Obj.(*networking_v1.Ingress)
		oldobj, _ = e.OldObj.(*networking_v1.Ingress)

		if !ok {
			fmt.Println("Error casting object to Ingress type")
			return &data
		}
	case "HorizontalPodAutoscaler":
		fmt.Println("autoscaling")
		var ok bool
		obj, ok = e.Obj.(*autoscaling_v2.HorizontalPodAutoscaler)
		oldobj, _ = e.OldObj.(*autoscaling_v2.HorizontalPodAutoscaler)

		if !ok {
			fmt.Println("Error casting object to HPA type")
			return &data
		}
		fmt.Println("autoscaling")
	case "DaemonSet":
		var ok bool
		obj, ok = e.Obj.(*apps_v1.DaemonSet)
		oldobj, _ = e.OldObj.(*apps_v1.DaemonSet)

		if !ok {
			fmt.Println("Error casting object to DaemonSet type")
			return &data
		}
	case "StatefulSet":
		var ok bool
		obj, ok = e.Obj.(*apps_v1.StatefulSet)
		oldobj, _ = e.OldObj.(*apps_v1.StatefulSet)

		if !ok {
			fmt.Println("Error casting object to StatefulSet type")
			return &data
		}
	case "Job":
		var ok bool
		obj, ok = e.Obj.(*batch_v1.Job)
		oldobj, _ = e.OldObj.(*batch_v1.Job)

		if !ok {
			fmt.Println("Error casting object to Job type")
			return &data
		}
	case "PersistentVolume":
		var ok bool
		obj, ok = e.Obj.(*api_v1.PersistentVolume)
		oldobj, _ = e.OldObj.(*api_v1.PersistentVolume)

		if !ok {
			fmt.Println("Error casting object to PersistentVolume type")
			return &data
		}
	case "Secret":
		var ok bool
		obj, ok = e.Obj.(*api_v1.Secret)
		oldobj, _ = e.OldObj.(*api_v1.Secret)

		if !ok {
			fmt.Println("Error casting object to Secret type")
			return &data
		}
	case "ConfigMap":
		var ok bool
		obj, ok = e.Obj.(*api_v1.ConfigMap)
		oldobj, _ = e.OldObj.(*api_v1.ConfigMap)

		if !ok {
			fmt.Println("Error casting object to ConfigMap type")
			return &data
		}
	case "ServiceAccount":
		var ok bool
		obj, ok = e.Obj.(*api_v1.ServiceAccount)
		oldobj, _ = e.OldObj.(*api_v1.ServiceAccount)

		if !ok {
			fmt.Println("Error casting object to ServiceAccount type")
			return &data
		}
	case "ClusterRole":
		var ok bool
		obj, ok = e.Obj.(*rbac_v1.ClusterRole)
		oldobj, _ = e.OldObj.(*rbac_v1.ClusterRole)

		if !ok {
			fmt.Println("Error casting object to ClusterRole type")
			return &data
		}
	case "ClusterRoleBinding":
		var ok bool
		obj, ok = e.Obj.(*rbac_v1.ClusterRoleBinding)
		oldobj, _ = e.OldObj.(*rbac_v1.ClusterRoleBinding)

		if !ok {
			fmt.Println("Error casting object to ClusterRoleBinding type")
			return &data
		}
	default:
		fmt.Println("Unhandled object kind:", e.Kind)
		return &data
	}
	if obj != nil && !reflect.ValueOf(obj).IsNil() {
		data.extractDetails(obj, &data.CurrentConfigName, &data.CurrentConfigNamespace, &data.CurrentConfigSpec, e.Kind)
	}
	if oldobj != nil && !reflect.ValueOf(oldobj).IsNil() {
		data.extractDetails(oldobj, &data.OldConfigName, &data.OldConfigNamespace, &data.OldConfigSpec, e.Kind)
	}
	return &data
}
func (data *objectData) extractDetails(obj interface{}, name *string, namespace *string, spec *interface{}, kind string) {
	if obj == nil {
		return
	}
	val := reflect.ValueOf(obj)

	switch val.Kind() {
	case reflect.Ptr:
		val = val.Elem()
	}

	nameField, ok := val.Type().FieldByName("Name")
	if !ok {
		return
	}
	*name = val.FieldByName(nameField.Name).String()
	namespaceField := val.FieldByName("Namespace")
	if namespaceField.IsValid() {
		*namespace = namespaceField.String()
	}
	if kind == "ConfigMap" || kind == "Secret" {
		specField := val.FieldByName("Data")
		if specField.IsValid() {
			*spec = specField.Interface()
		}
	}
	if kind == "ClusterRole" {
		specField := val.FieldByName("Rules")
		if specField.IsValid() {
			*spec = specField.Interface()
		}
	}
	if kind == "ClusterRoleBinding" {
		subjectsField := val.FieldByName("Subjects")
		roleRefField := val.FieldByName("RoleRef")
		if subjectsField.IsValid() && roleRefField.IsValid() {
			combinedSpec := fmt.Sprintf("Rules: %v, RoleRef: %v", subjectsField.Interface(), roleRefField.Interface())
			*spec = combinedSpec
		}
	} else {
		specField := val.FieldByName("Spec")
		if specField.IsValid() {
			*spec = specField.Interface()
		}
	}
}
func extractLabels(e event.Event, label string) string {
	eventLabels := utils.GetObjectMetaData(e.Obj)
	if eventLabels.Labels != nil {
		return eventLabels.Labels[label]
	}
	return ""
}

func getActionBy(kind string) string {
	switch kind {
	case "deployment":
		return "deployment-controller"
	case "service":
		return "service-controller"
	case "configmap":
		return "configmap-controller"
	case "secret":
		return "secret-controller"
	case "namespace":
		return "namespace-controller"
	case "deamonset":
		return "deamonset-controller"
	case "statefulset":
		return "statefulset-controller"
	case "ingress":
		return "ingress-controller"
	default:
		return "kubernetes-controller"
	}
}

func getRiskLevel(kind string) string {
	switch kind {
	case "deployment":
		return RiskLevelMedium
	case "service":
		return RiskLevelHigh
	case "configmap":
		return RiskLevelMedium
	case "secret":
		return RiskLevelMedium
	case "namespace":
		return RiskLevelHigh
	case "deamonset":
		return RiskLevelLow
	case "statefulset":
		return RiskLevelHigh
	case "ingress":
		return RiskLevelHigh
	default:
		return RiskLevelMedium
	}
}

func postMessage(url string, webhookMessage *CustomWebhookMessage) error {
	message, err := json.Marshal(webhookMessage)
	if err != nil {
		return err
	}
	fmt.Println("*************")
	fmt.Println(bytes.NewBuffer(message))
	fmt.Println("*************")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(message))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}
