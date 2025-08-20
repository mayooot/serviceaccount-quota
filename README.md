# serviceaccount-quota



# Quick Start

### Local Development

```bash
make generate
make install

make docker-build
make deploy
```

```bash
$ kubectl apply -f config/samples/quota_v1alpha1_serviceaccountquota.yaml

$ kubectl get serviceaccountquotas.quota.mayooot.github.io               
NAME                         SERVICEACCOUNT   QUOTA                                                                                                                                          LASTRECONCILED   AGE
serviceaccountquota-sample   yoshi            limits.cpu: 0/800, limits.memory: 0/1600Gi, limits.nvidia.com: 0/4, requests.cpu: 0/800, requests.memory: 0/1600Gi, requests.nvidia.com: 0/4   8s               8s

```