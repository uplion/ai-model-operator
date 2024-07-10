package listener

import (
	"context"
	"fmt"
	modelv1alpha1 "github.com/uplion/aimodel-operator/api/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func StartEventListener(kubeClient kubernetes.Interface, dynamicClient client.Client, namespace string) {
	log.Println("Starting event listener")

	watcher, err := kubeClient.CoreV1().Events(v1.NamespaceAll).Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to create watcher: %v", err)
		return
	}

	log.Println("Watcher created")

	startTime := time.Now()

	for event := range watcher.ResultChan() {
		if event.Type == watch.Added && event.Object.(*v1.Event).CreationTimestamp.Time.After(startTime) {
			handleEvent(event.Object.(*v1.Event), dynamicClient)
		}
	}
}

func handleEvent(event *v1.Event, dynamicClient client.Client) {
	if event.InvolvedObject.Kind == "AIModel" && event.Type == v1.EventTypeWarning {
		log.Printf("Handling event: %s %s", event.InvolvedObject.Name, event.Message)
		updateAIModelStatus(event, dynamicClient)
	}
}

func updateAIModelStatus(event *v1.Event, dynamicClient client.Client) {
	ctx := context.Background()

	aimodel := &modelv1alpha1.AIModel{}
	err := dynamicClient.Get(ctx, client.ObjectKey{
		Namespace: event.InvolvedObject.Namespace,
		Name:      event.InvolvedObject.Name,
	}, aimodel)
	if err != nil {
		log.Printf("Failed to get AIModel: %v", err)
		return
	}

	aimodel.Status.State = "Failed"
	aimodel.Status.Message = fmt.Sprintf("%s: %s", event.Reason, event.Message)

	// Debug logging to verify status before update
	log.Printf("Updating AIModel status: State=%s, Message=%s", aimodel.Status.State, aimodel.Status.Message)
	err = dynamicClient.Status().Update(ctx, aimodel)
	if err != nil {
		log.Printf("Failed to update AIModel status: %v", err)
	}
}
