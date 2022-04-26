package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	secretPath     = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath = filepath.Join("testdata", "kubeconfig.yaml")
	deploymentPath = filepath.Join("testdata", "kcmdeployment.yaml")
	ctx            context.Context
	deployment     appsv1.Deployment
	secret         corev1.Secret
	k8sClient      client.Client
	testEnv        *envtest.Environment
	cfg            *rest.Config
	err            error
)

func BeforeSuite(t *testing.T) {
	t.Log("setting up envTest")
	fileExistsOrFail(secretPath)
	fileExistsOrFail(deploymentPath)
	fileExistsOrFail(kubeConfigPath)
	testEnv = &envtest.Environment{}
	cfg, err = testEnv.Start()
	if err != nil {
		log.Fatalf("error in starting testEnv: %v", err)
	}
	if cfg == nil {
		log.Fatalf("Got nil config from testEnv")
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Fatalf("error in creating new client: %v", err)
	}
	if k8sClient == nil {
		log.Fatalf("Got a nil k8sClient")
	}
}

func AfterSuite(t *testing.T) {
	log.Println("tearing down envTest")
	err := testEnv.Stop()
	if err != nil {
		log.Fatalf("error in stopping testEnv: %v", err)
	}
}

func TestSuite(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"Secret not found", testSecretNotFound},
		{"Kubeconfig not found", testKubeConfigNotFound},
		{"Extract Kubeconfig from secret", testExtractKubeConfigFromSecret},
		{"Deployment not found ", testDeploymentNotFound},
		{"Deployment is found", testFoundDeployment},
		{"Create Scales Getter", testCreateScalesGetter},
		{"Create client from kubeconfig", testCreateClientFromKubeconfigBytes},
	}
	BeforeSuite(t)
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
	AfterSuite(t)
}

func testSecretNotFound(t *testing.T) {
	g := NewWithT(t)
	_ = setupGetKubeconfigTest(t, k8sClient)
	kubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	g.Expect(kubeconfig).Should(BeNil())
}

func testKubeConfigNotFound(t *testing.T) {
	g := NewWithT(t)
	cleanup := setupGetKubeconfigTest(t, k8sClient)
	defer cleanup()
	err := k8sClient.Create(ctx, &secret)
	g.Expect(err).Should(BeNil())
	kubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
	g.Expect(kubeconfig).Should(BeNil())
	g.Expect(err).ShouldNot(BeNil())
	g.Expect(apierrors.IsNotFound(err)).Should(BeFalse())
}

func testExtractKubeConfigFromSecret(t *testing.T) {
	g := NewWithT(t)
	cleanup := setupGetKubeconfigTest(t, k8sClient)
	defer cleanup()
	kubeconfigBuffer, err := readFile(kubeConfigPath)
	g.Expect(err).Should(BeNil())
	kubeconfig := kubeconfigBuffer.Bytes()
	g.Expect(kubeconfig).ShouldNot(BeNil())

	secret.Data = map[string][]byte{
		"kubeconfig": kubeconfig,
	}
	err = k8sClient.Create(ctx, &secret)
	g.Expect(err).Should(BeNil())

	actualKubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
	g.Expect(err).Should(BeNil())
	g.Expect(actualKubeconfig).Should(Equal(kubeconfig))
}

func testDeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	setupGetDeploymentTest(t)
	actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient)
	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	g.Expect(actual).Should(BeNil())
}

func testFoundDeployment(t *testing.T) {
	g := NewWithT(t)
	setupGetDeploymentTest(t)

	err := k8sClient.Create(ctx, &deployment)
	g.Expect(err).Should(BeNil())

	actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient)
	g.Expect(err).Should(BeNil())
	g.Expect(actual).ShouldNot(BeNil())
	g.Expect(actual.ObjectMeta.Name).Should(Equal(deployment.ObjectMeta.Name))
	g.Expect(actual.ObjectMeta.Namespace).Should(Equal(deployment.ObjectMeta.Namespace))

	err = k8sClient.Delete(ctx, &deployment)
	g.Expect(err).Should(BeNil())
}

func testCreateScalesGetter(t *testing.T) {
	g := NewWithT(t)
	scaleGetter, err := CreateScalesGetter(cfg)
	g.Expect(err).Should(BeNil())
	g.Expect(scaleGetter).ShouldNot(BeNil())
}

func testCreateClientFromKubeconfigBytes(t *testing.T) {
	g := NewWithT(t)
	kubeconfigBuffer, err := readFile(kubeConfigPath)
	g.Expect(err).Should(BeNil())
	kubeconfig := kubeconfigBuffer.Bytes()
	g.Expect(kubeconfig).ShouldNot(BeNil())

	cfg, err := CreateClientFromKubeConfigBytes(kubeconfig)
	g.Expect(err).Should(BeNil())
	g.Expect(cfg).ShouldNot(BeNil())
}

func setupGetKubeconfigTest(t *testing.T, k8sClient client.Client) func() {
	g := NewWithT(t)
	ctx = context.Background()
	result := getStructured[corev1.Secret](secretPath)
	g.Expect(result.Err).Should(BeNil())
	g.Expect(result.StructuredObject).ShouldNot(BeNil())
	secret = result.StructuredObject

	return func() {
		err := k8sClient.Delete(ctx, &secret)
		g.Expect(err).Should(BeNil())
	}
}

func setupGetDeploymentTest(t *testing.T) {
	g := NewWithT(t)
	ctx = context.Background()
	result := getStructured[appsv1.Deployment](deploymentPath)
	g.Expect(result.Err).Should(BeNil())
	g.Expect(result.StructuredObject).ShouldNot(BeNil())
	deployment = result.StructuredObject
}

type Result[T any] struct {
	StructuredObject T
	Err              error
}

func getStructured[T any](filepath string) Result[T] {
	unstructuredObject, err := getUnstructured(filepath)
	if err != nil {
		return Result[T]{Err: err}
	}
	var structuredObject T
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObject.Object, &structuredObject)
	if err != nil {
		return Result[T]{Err: err}
	}
	return Result[T]{StructuredObject: structuredObject}
}

func getUnstructured(filePath string) (*unstructured.Unstructured, error) {
	buff, err := readFile(filePath)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}
	jsonObject, err := yaml.ToJSON(buff.Bytes())
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsonObject)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}
	unstructuredObject, ok := object.(*unstructured.Unstructured)
	if !ok {
		return &unstructured.Unstructured{}, fmt.Errorf("unstructured.Unstructured expected")
	}
	return unstructuredObject, nil
}

func readFile(filePath string) (*bytes.Buffer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(file)
	if err != nil {
		return nil, err
	}
	return buff, nil
}

func fileExistsOrFail(filepath string) {
	var err error
	if _, err = os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", filepath)
	}
	if err != nil {
		log.Fatalf("Error occured in finding file %s : %v", filepath, err)
	}
}
