# AI Model Operator

AI Model Operator is a Kubernetes operator designed specifically for the uplion project, used to create and manage AI models.

## Description

Create an AIModel resource similar to the following. The AIModel resource defines the deployment information for an AI model, including the model name, model type, model image, number of model replicas, etc.

```yaml
apiVersion: model.youxam.com/v1alpha1
kind: AIModel
metadata:
  name: ai-model-sample
spec:
  type: local
  model: TinyLlama-1.1B
  replicas: 3
  image: user/image:tag
  maxProcessNum: 256
```

You can also create a remote model:

```yaml
apiVersion: model.youxam.com/v1alpha1
kind: AIModel
metadata:
  name: ai-model-sample
spec:
  type: remote
  model: gpt-3.5-turbo
  apiKey: xxxxxxx
  baseURL: https://api.openai.com
  replicas: 3
  image: user/image:tag
  maxProcessNum: 256
```

Taking the local model as an example:

```bash
$ kubectl apply -f demo.yaml
aimodel.model.youxam.com/ai-model-sample created
```

At this point, the AI model operator will create corresponding deployments and pods based on the AIModel resource definition:

```
$ kubectl get pods,deploy,aimodel
NAME                                              READY   STATUS    RESTARTS   AGE
pod/ai-model-sample-deployment-8464ffcfff-6xh7g   1/1     Running   0          29s
pod/ai-model-sample-deployment-8464ffcfff-ccf9q   1/1     Running   0          29s
pod/ai-model-sample-deployment-8464ffcfff-rwzk8   1/1     Running   0          29s

NAME                                         READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/ai-model-sample-deployment   3/3     3            3           29s

NAME                                       TYPE    MODEL            REPLICAS   STATE
aimodel.model.youxam.com/ai-model-sample   local   TinyLlama-1.1B   3          Running
```

If an irreversible error occurs, such as a configuration error, it will also be reflected in the status of the AIModel resource:

```bash
# Trigger an error
$ kubectl get aimodel
NAME              TYPE    MODEL            REPLICAS   STATE
ai-model-sample   local   TinyLlama-1.1B   3          Failed
$ kubectl describe aimodel
Name:         ai-model-sample
Namespace:    default
Labels:       app=ai-model
Annotations:  <none>
API Version:  model.youxam.com/v1alpha1
Kind:         AIModel
Metadata:
  Creation Timestamp:  2024-07-10T17:28:45Z
  Generation:          2
  Resource Version:    133511
  UID:                 2337a795-c15a-4401-a545-be4750fdff42
Spec:
  Image:     youxam/uplion-aimodel-operator-test-worker:latest
  Model:     TinyLlama-1.1B
  Replicas:  3
  Type:      local
Status:
  Message:  ConfigurationError: Local model "not-found-model" is not supported.
  State:    Failed
Events:     <none>
```

The status of `ai-model-sample` is `Failed`, and there is an error message `ConfigurationError: Local model "not-found-model" is not supported.`

At this point, the status of the corresponding pod depends on the program logic. If the program continues to run after reporting the error, the pod will still be in the `Running` state, but service availability is not guaranteed. If the program exits after reporting the error, the pod will enter the `CrashLoopBackOff` state.

### KEDA

The AI Model Operator supports KEDA. You can use KEDA to scale the number of replicas based on the number of messages in the message queue. For example, the following configuration scales the number of replicas based on the number of messages in the message queue:

```yaml
apiVersion: model.youxam.com/v1alpha1
kind: AIModel
metadata:
  name: ai-model-sample
spec:
  type: local
  model: TinyLlama-1.1B
  msgBacklogThreshold: 2 # The number of messages in the message queue that triggers scaling
  image: user/image:tag
  maxProcessNum: 256
```

```bash
$ kubectl apply -f demo.yaml
$ kubectl get scaledobject
NAME                           SCALETARGETKIND      SCALETARGETNAME              MIN   MAX   TRIGGERS   AUTHENTICATION   READY   ACTIVE    FALLBACK   PAUSED    AGE
ai-model-sample-scaledobject   apps/v1.Deployment   ai-model-sample-deployment   1           pulsar                      True    Unknown   Unknown    Unknown   2s
$ kubectl get hpa
NAME                                    REFERENCE                               TARGETS             MINPODS   MAXPODS   REPLICAS   AGE
keda-hpa-ai-model-sample-scaledobject   Deployment/ai-model-sample-deployment   3/2 (avg)   1         100       0          3s
```

## Environment Variables

The Operator supports the following environment variables:

1. `PULSAR_ADMIN_URL`: The URL of Pulsar Admin, default is "";
2. `PULSAR_URL`: The URL of Pulsar, default is `pulsar://localhost:6650`;
3. `PULSAR_TOKEN`: The Token of Pulsar, default is empty;
4. `RES_TOPIC_NAME`: The Topic name of the result message queue, default is `res-topic`;

All the above environment variables will be passed to the Pod except `PULSAR_ADMIN_URL`.

In addition, the Operator will automatically inject the following environment variables:

1. `NODE_TYPE`: Node type, value is either `local` or `remote`, corresponding to the `type` in the yaml file
2. `MODEL_NAME`: Model name, corresponding to `model`
3. `API_URL`: API URL for remote models, corresponding to `baseURL`
4. `API_KEY`: API Key for remote models, corresponding to `apiKey`
5. `MAX_PROCESS_NUM`: Maximum number of threads that can be used to process messages, corresponding to `maxProcessNum`
6. `AIMODEL_NAME`: The name of the AIModel resource, in the above example it's `ai-model-sample`
7. `AIMODEL_NAMESPACE`: The namespace where the AIModel resource is located, in the above example it's `default`

`AIMODEL_NAME` and `AIMODEL_NAMESPACE` are provided for the program inside the Pod to report status, as described below.

## Reporting Irreversible Errors

The program inside the Pod may encounter irreversible errors, such as:

1. `ConfigurationError`: When the program inside the Pod discovers a configuration error, it should report a `ConfigurationError`. For example, `Local model "not-found-model" is not supported.` in the above example;
2. `AuthenticationError`: When the program inside the Pod believes that API Key authentication has failed, it should report an `AuthenticationError`;
3. `MessageQueueConnectionError`: When the program inside the Pod cannot connect to the message queue, it should report a `MessageQueueConnectionError`;
4. `APIQuotaExceededError`: When the program inside the Pod believes that the API quota has been consumed, it should report an `APIQuotaExceededError`;
5. `GeneralError`: Other errors.

At this time, the internal program should use Kubernetes' event mechanism to report errors. The `Reason` field should be one of the above error types, and the `Message` field should be a human-readable error message.

The Operator will automatically assign the permission to create events to the Pod, and listen for events that meet the following conditions:

1. `Type` is `Warning`;
2. `InvolvedObject.Kind` is `AIModel`;
3. `InvolvedObject.Name` is the name of an existing AIModel resource;
4. `InvolvedObject.Namespace` is the namespace of an existing AIModel resource.

If the Pod reports an event that meets the above conditions, the Operator will update the status of the corresponding AIModel resource to `Failed` and record the error message in the `Status.Message` field.

When the AIModel is modified, the status will be reset to `Running`. If the program inside the Pod continues to report errors, the status will change to `Failed` again.

For example, Golang code, see the [createK8sEvent function](https://github.com/uplion/ai-model-operator/blob/371c44df88e85d336638f268f370d11805458899/worker/main.go#L54-L103).

## Installation

### Prerequisites
- go version v1.20.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/aimodel-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples have default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

