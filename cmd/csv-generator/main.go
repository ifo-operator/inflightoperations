/*
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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blang/semver/v4"
	"github.com/ifo-operator/inflightoperations/settings"
	"github.com/operator-framework/api/pkg/lib/version"
	csvv1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

type generatorFlags struct {
	file            string
	crds            string
	dumpCRDs        bool
	csvVersion      string
	namespace       string
	operatorImage   string
	operatorVersion string
	pullPolicy      string
}

var (
	flags   generatorFlags
	command = &cobra.Command{
		Use:   "csv-generator",
		Short: "csv-generator for inflightoperations",
		Long:  `csv-generator generates deploy manifest for inflightoperations`,
		Run: func(cmd *cobra.Command, args []string) {
			err := Generate()
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
	}
)

func init() {
	command.Flags().StringVar(&flags.file, "file", "bundle/manifests/inflightoperations.clusterserviceversion.yaml", "Location of the CSV yaml to modify")
	command.Flags().StringVar(&flags.crds, "crds", "config/crd/bases", "Location of the CRD files")
	command.Flags().StringVar(&flags.csvVersion, "csv-version", "", "Version of csv manifest (required)")
	command.Flags().StringVar(&flags.namespace, "namespace", "", "Namespace in which ssp operator will be deployed (required)")
	command.Flags().StringVar(&flags.operatorImage, "operator-image", "", "Link to operator image (required)")
	command.Flags().StringVar(&flags.operatorVersion, "operator-version", "", "Operator version (required)")
	command.Flags().StringVar(&flags.pullPolicy, "pull-policy", "IfNotPresent", "Image pull policy")
	command.Flags().BoolVar(&flags.dumpCRDs, "dump-crds", false, "Dump crds to stdout")

	if err := command.MarkFlagRequired("csv-version"); err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	if err := command.MarkFlagRequired("namespace"); err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	if err := command.MarkFlagRequired("operator-image"); err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	if err := command.MarkFlagRequired("operator-version"); err != nil {
		panic(fmt.Sprintf("%v", err))
	}
}

func main() {
	err := command.Execute()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func Generate() (err error) {
	csvFile, err := os.Open(flags.file)
	if err != nil {
		return
	}
	defer func() {
		_ = csvFile.Close()
	}()
	decoder := yaml.NewYAMLOrJSONDecoder(csvFile, 1024)
	csv := csvv1.ClusterServiceVersion{}
	err = decoder.Decode(&csv)
	if err != nil {
		return
	}
	err = modifyManagerDeployment(&csv)
	if err != nil {
		return
	}
	err = marshalCSV(csv, os.Stdout)
	if err != nil {
		return
	}
	if flags.dumpCRDs {
		err = dumpCRDs(flags.crds)
		if err != nil {
			return
		}
	}
	return
}

func modifyManagerDeployment(csv *csvv1.ClusterServiceVersion) (err error) {
	csv.Name = "inflightoperations.v" + flags.csvVersion
	v, err := semver.New(flags.csvVersion)
	if err != nil {
		return
	}
	csv.Spec.Version = version.OperatorVersion{
		Version: *v,
	}
	templateSpec := &csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs[0].Spec.Template.Spec
	if len(templateSpec.Containers) != 1 {
		err = fmt.Errorf("expected exactly one container")
		return
	}
	container := templateSpec.Containers[0]
	container.Image = flags.operatorImage
	container.ImagePullPolicy = v1.PullPolicy(flags.pullPolicy)
	container.Args = append(container.Args, "--olm-deployment")
	for i := range container.Env {
		envVar := &container.Env[i]
		switch envVar.Name {
		case settings.EnvOperatorVersion:
			if flags.operatorVersion != "" {
				envVar.Value = flags.operatorVersion
			}
		}
	}
	templateSpec.Containers[0] = container
	return
}

func marshalCSV(obj any, writer io.Writer) (err error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	var r unstructured.Unstructured
	err = json.Unmarshal(jsonBytes, &r.Object)
	if err != nil {
		return err
	}
	trimMetadata(&r)
	deployments, exists, err := unstructured.NestedSlice(r.Object, "spec", "install", "spec", "deployments")
	if err != nil {
		return err
	}
	if exists {
		for _, d := range deployments {
			deployment := d.(map[string]any)
			unstructured.RemoveNestedField(deployment, "metadata", "creationTimestamp")
			unstructured.RemoveNestedField(deployment, "spec", "template", "metadata", "creationTimestamp")
			unstructured.RemoveNestedField(deployment, "status")
		}
		if err = unstructured.SetNestedSlice(r.Object, deployments, "spec", "install", "spec", "deployments"); err != nil {
			return err
		}
	}
	err = writeDocument(&r, writer)
	if err != nil {
		return
	}
	return
}

func dumpCRDs(path string) (err error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, file := range files {
		err = dumpCRD(filepath.Join(path, file.Name()))
		if err != nil {
			return
		}
	}
	return
}

func dumpCRD(path string) (err error) {
	var crdFile *os.File
	crdFile, err = os.Open(path)
	if err != nil {
		return
	}
	defer func() {
		_ = crdFile.Close()
	}()
	var obj unstructured.Unstructured
	decoder := yaml.NewYAMLOrJSONDecoder(crdFile, 1024)
	err = decoder.Decode(&obj)
	if err != nil {
		return
	}
	trimMetadata(&obj)
	err = writeDocument(&obj, os.Stdout)
	if err != nil {
		return
	}
	return
}

func trimMetadata(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "template", "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "spec", "template", "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "status")
}

func writeDocument(obj *unstructured.Unstructured, writer io.Writer) (err error) {
	yamlBytes, err := sigsyaml.Marshal(obj.Object)
	if err != nil {
		return
	}
	_, err = writer.Write([]byte("---\n"))
	if err != nil {
		return
	}
	_, err = writer.Write(yamlBytes)
	if err != nil {
		return
	}
	return
}
