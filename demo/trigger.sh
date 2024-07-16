#!/bin/bash
echo -e "\033[33mTriggering an error...\033[0m"
reason=${1:-"Unknown reason"}
message=${2:-"Unknown message"}
echo -e "\033[31m\t$reason: $message\033[0m"
kubectl apply -f service.yaml > /dev/null
sleep 1
kubectl port-forward svc/ai-model-svc 9999:8080 > /dev/null &
bg_pid=$!
sleep 0.5
trap "kill $bg_pid; exit" EXIT
curl -s "http://localhost:9999/?reason=$reason&message=$message" | jq -r '.message + " " + .status'
kubectl delete -f service.yaml > /dev/null