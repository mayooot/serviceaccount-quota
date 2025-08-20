package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	nodeName := flag.String("node", "orbstack", "Name of the node to add GPU resources")
	gpuName := flag.String("gpu-name", "nvidia.com/gpu", "Name of the GPU resource (e.g., nvidia.com/gpu)")
	gpuCount := int64Flag("count", 8, "Number of GPU cards to simulate")
	flag.Parse()

	if *nodeName == "" {
		fmt.Println("Error: node name cannot be empty")
		return
	}
	if *gpuName == "" {
		fmt.Println("Error: GPU name cannot be empty")
		return
	}
	if *gpuCount <= 0 {
		fmt.Println("Error: GPU count must be positive")
		return
	}

	clientset, err := initKubeClient()
	if err != nil {
		fmt.Printf("Error initializing Kubernetes client: %v\n", err)
		return
	}

	ctx := context.Background()
	_, err = clientset.CoreV1().Nodes().Get(ctx, *nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Error getting node %s: %v\n", *nodeName, err)
		return
	}

	gpuPath := strings.ReplaceAll(*gpuName, "/", "~1")
	patch := []map[string]interface{}{
		{
			"op":    "add",
			"path":  fmt.Sprintf("/status/capacity/%s", gpuPath),
			"value": fmt.Sprintf("%d", *gpuCount),
		},
		{
			"op":    "add",
			"path":  fmt.Sprintf("/status/allocatable/%s", gpuPath),
			"value": fmt.Sprintf("%d", *gpuCount),
		},
	}

	bytes, _ := json.Marshal(patch)
	_, err = clientset.CoreV1().Nodes().Patch(ctx, *nodeName, types.JSONPatchType, bytes, metav1.PatchOptions{}, "status")
	if err != nil {
		fmt.Printf("Error patching node %s: %v\n", *nodeName, err)
		return
	}

	fmt.Printf("Successfully added %d %s to node %s\n", *gpuCount, *gpuName, *nodeName)
}

func initKubeClient() (*kubernetes.Clientset, error) {
	kubeconfig := ""
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func int64Flag(name string, value int64, usage string) *int64 {
	p := new(int64)
	*p = value
	flag.Int64Var(p, name, value, usage)
	return p
}
