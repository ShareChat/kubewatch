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
	"strings"

	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	apps_v1 "k8s.io/api/apps/v1"

	"github.com/bitnami-labs/kubewatch/config"
	"github.com/bitnami-labs/kubewatch/pkg/event"
	"github.com/bitnami-labs/kubewatch/pkg/utils"
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
	return object.Generation != oldObject.Generation
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
	riskLevel := getRiskLevel(kind)
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
		RiskLevel:   riskLevel,
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

	object, ok := e.Obj.(*apps_v1.Deployment)
	if !ok {
		fmt.Println("Error casting oldConfig to Deployment type")
		return &data
	}
	oldObj := e.OldObj.(*apps_v1.Deployment)
	if !ok {
		fmt.Println("Error casting currentConfig to Deployment type")
		return &data
	}

	data.CurrentConfigName = object.GetName()
	data.CurrentConfigNamespace = object.GetNamespace()
	data.CurrentConfigSpec = object.Spec

	data.OldConfigName = oldObj.GetName()
	data.OldConfigNamespace = oldObj.GetNamespace()
	data.OldConfigSpec = oldObj.Spec

	return &data
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
