#!/usr/bin/env bash
set -e

SVCS=`kubectl get service -o json | jq -r '.items | map(select(.metadata.ownerReferences)) | map(select(any(.metadata.ownerReferences[]; .kind == "Stack") and .spec.clusterIP != "None") | "\(.metadata.name):\(.spec.ports[0].port)" ) | @sh '`

SUDOMESSAGE="Please enter your sudo password to modify the hosts file tee"

echo > portfwd.log
for i in $SVCS; do
    eval NAMEPORT=$i
    SVC=${NAMEPORT%:*}
    FQN=$SVC.default.svc.cluster.local    
    if ! grep "$FQN" /etc/hosts > /dev/null; then
       echo "127.0.0.1   $FQN" | sudo -p "${SUDOMESSAGE}" tee -a /etc/hosts > /dev/null
    fi
     
	nohup kubectl port-forward service/$SVC ${NAMEPORT#*:} >> portfwd.log &
done
