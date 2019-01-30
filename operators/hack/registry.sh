#!/bin/bash -eu

help() { 
  echo \
'Manage a Docker registry in k8s.

Usage:
  registry.sh [command]

Commands:
    create              Deploy the registry k8s resources
    port-forward start  Start to forward the localhost port 5000 to the registry port 5000
    port-forward stop   Stop the port forwarding
    delete              Delete the registry k8s resources
    start               Execute the `create` and `port-forward start` commands'
}

main() {
  case $@ in
    start)
      main create
      main port-forward start
    ;;
    create)
      kubectl apply -f config/dev/registry.yaml
    ;;
    "port-forward start")
      kubectl wait pods -l k8s-app=kube-registry -n=kube-system --for condition=Ready --timeout 10s
      local podName=$(kubectl get po -n kube-system | grep kube-registry-v0 | awk '{print $1}')
      kubectl port-forward -n=kube-system $podName 5000:5000 > /dev/null &
      timeout 15 sh -c 'until nc -z localhost 5000; do sleep 0.5; done'
    ;;
    "port-forward stop")
      local pid=$(ps aux | grep 'kubectl port-forward.*registry' | grep -v grep | awk '{print $2}')
      kill $pid
    ;;
    delete)
      kubectl delete -f config/dev/registry.yaml
    ;;
    *)
      help; exit 1
    ;;
  esac
}

main "$@"