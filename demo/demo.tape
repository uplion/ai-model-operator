Output demo.mp4

Set FontSize 36
Set Width 2400
Set Height 1000
Set Shell zsh
Set FontFamily "MesloLGS NF"
Set TypingSpeed 0.1


Hide
Type "source ~/.zshrc"
Enter
Sleep 1s
Type "clear"
Enter
Sleep 1s
Show


Type "# There are currently no objects in the cluster."
Enter
Sleep 1s

Type "kubectl get aimodel,pod,deploy"
Enter
Sleep 4s

Type "bat demo.yaml"
Enter
Sleep 5s

Type "# Now create one"
Enter
Sleep 1s
Type "kubectl apply -f demo.yaml"
Enter
Sleep 1s

Type "# Now there is one AI model with three corresponding pods."
Enter
Sleep 3s
Type "kubectl get aimodel,pod,deploy"
Enter
Sleep 6s

Type "# Set replicas to 2."
Enter
Sleep 1s
Type "yq eval '.spec.replicas = 2' demo.yaml -i && bat demo.yaml"
Enter
Sleep 3s

Type "# Now update the ai model"
Enter
Sleep 1s
Type "kubectl apply -f demo.yaml"
Enter
Sleep 1s

Type "# Now there are 2 pods corresponding to the ai model."
Enter
Sleep 3s
Type "kubectl get aimodel,pod,deploy"
Enter
Sleep 4s


Type "# The status of the ai model should be 'Running'."
Enter
Sleep 2s
Type "kubectl get aimodel ai-model-sample -o=jsonpath='{.status}' | jq"
Enter
Sleep 2s

Type "# Trigger an error"
Enter
Sleep 1s
Type "./trigger.sh ConfigurationError some_error"
Enter
Sleep 3s

Type "# The status of the ai model should be 'Failed'."
Enter
Sleep 2s
Type "kubectl get aimodel ai-model-sample -o=jsonpath='{.status}' | jq"
Enter
Sleep 2s

Hide
Type "kubectl delete -f demo.yaml"
Enter
Sleep 2s
Type "yq eval '.spec.replicas = 3' demo.yaml -i"
Enter
Sleep 1s