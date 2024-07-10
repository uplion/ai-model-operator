package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func printEnvVariables() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func main() {
	// Print all environment variables
	printEnvVariables()

	// Create Gin router
	r := gin.Default()

	// Define GET / route
	r.GET("/", func(c *gin.Context) {
		reason := c.Query("reason")
		message := c.Query("message")

		// Create Kubernetes event
		err := createK8sEvent(reason, message)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Event created",
		})
	})

	// Run the Gin server
	r.Run()
}

func createK8sEvent(reason, message string) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("Error creating in-cluster config: %v\n", err)
		return fmt.Errorf("error creating in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating clientset: %v\n", err)
		return fmt.Errorf("error creating clientset: %v", err)
	}

	namespace := os.Getenv("AIMODEL_NAMESPACE")
	aimodelName := os.Getenv("AIMODEL_NAME")

	if namespace == "" || aimodelName == "" {
		fmt.Printf("Namespace or AIModel name not set\n")
		return fmt.Errorf("namespace or AIModel name not set")
	}

	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: aimodelName + "-error-",
			Namespace:    namespace,
		},
		InvolvedObject: v1.ObjectReference{
			Kind:      "AIModel",
			Namespace: namespace,
			Name:      aimodelName,
		},
		Reason:  reason,
		Message: message,
		Source: v1.EventSource{
			Component: "worker",
		},
		Type:           v1.EventTypeWarning,
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
		Count:          1,
	}

	_, err = clientset.CoreV1().Events(namespace).Create(context.TODO(), event, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Error creating event: %v\n", err)
		return fmt.Errorf("error creating event: %v", err)
	}

	return nil
}
