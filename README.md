i![ci](https://github.com/doitintl/kubeip/workflows/ci/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/doitintl/kubeip)](https://goreportcard.com/report/github.com/doitintl/kubeip) ![Docker Pulls](https://img.shields.io/docker/pulls/doitintl/kubeip)

# What is kubeIP?

Many applications need to be whitelisted by users based on a Source IP Address. As of today, Google Kubernetes Engine doesn't support assigning a static pool of IP addresses to the GKE cluster. Using kubeIP, this problem is solved by assigning GKE nodes external IP addresses from a predefined list. kubeIP monitors the Kubernetes API for new/removed nodes and applies the changes accordingly.

# Deploy kubeIP (without building from source)

If you just want to use kubeIP (instead of building it yourself from source), please follow the instructions in this section. You’ll need Kubernetes version 1.10 or newer. You'll also need the Google Cloud SDK. You can install the [Google Cloud SDK](https://cloud.google.com/sdk) (which also installs kubectl).

To configure your Google Cloud SDK, set default project as:

```
gcloud config set project {your project_id}
```

Set the environment variables and make sure to configure before continuing:

```
export GCP_REGION=<gcp-region>
export GCP_ZONE=<gcp-zone>
export GKE_CLUSTER_NAME=<cluster-name>
export PROJECT_ID=$(gcloud config list --format 'value(core.project)')
export KUBEIP_NODEPOOL=<nodepool-with-static-ips>
export KUBEIP_SELF_NODEPOOL=<nodepool-for-kubeip-to-run-in>
```

**Creating an IAM Service Account and obtaining the Key in JSON format**

Create a Service Account with this command:

```
gcloud iam service-accounts create kubeip-service-account --display-name "kubeIP"
```

Create and attach a custom kubeIP role to the service account by running the following commands:

```
gcloud iam roles create kubeip --project $PROJECT_ID --file roles.yaml

gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member=serviceAccount:kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com \
    --role=projects/$PROJECT_ID/roles/kubeip \
    --condition=None
```

Generate the Key using the following command:

```
gcloud iam service-accounts keys create key.json \
    --iam-account kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com
```

**Create Kubernetes Secret Objects**

Get your GKE cluster credentaials with (replace `$GKE_CLUSTER_NAME` with your real GKE cluster name):

```
gcloud container clusters get-credentials $GKE_CLUSTER_NAME \
    --region $GCP_ZONE \
    --project $PROJECT_ID
```

Create a Kubernetes secret object by running:

```
kubectl create secret generic kubeip-key --from-file=key.json -n kube-system
```
Get RBAC permissions with:
```
kubectl create clusterrolebinding cluster-admin-binding \
    --clusterrole cluster-admin --user `gcloud config list --format 'value(core.account)'`
```
**Create Static, Reserved IP Addresses:**

Create as many static IP addresses for the number of nodes in your GKE cluster (this example creates 10 addresses) so you will have enough addresses when your cluster scales up (manually or automatically):

```
for i in {1..10}; do gcloud compute addresses create kubeip-ip$i --project=$PROJECT_ID --region=$GCP_REGION; done
```

Add labels to reserved IP addresses. A common practice is to assign a unique value per cluster (for example cluster name):

```
for i in {1..10}; do gcloud beta compute addresses update kubeip-ip$i --update-labels kubeip=$GKE_CLUSTER_NAME --region $GCP_REGION; done
```

```
sed -i -e "s/reserved/$GKE_CLUSTER_NAME/g" -e "s/default-pool/$KUBEIP_NODEPOOL/g" deploy/kubeip-configmap.yaml
```

Make sure the `deploy/kubeip-configmap.yaml` file contains the correct values:

 - The `KUBEIP_LABELVALUE` should be your GKE's cluster name
 - The `KUBEIP_NODEPOOL` should match the name of your GKE node-pool on which kubeIP will operate
 - The `KUBEIP_FORCEASSIGNMENT` - controls whether kubeIP should assign static IPs to existing nodes in the node-pool and defaults to true

We recommend that KUBEIP_NODEPOOL should *NOT* be the same as KUBEIP_SELF_NODEPOOL


If you would like to assign addresses to other node pools, then `KUBEIP_NODEPOOL` can be added to this nodepool `KUBEIP_ADDITIONALNODEPOOLS` as a comma separated list.
You should tag the addresses for this pool with the `KUBEIP_LABELKEY` value + `-node-pool` and assign the value of the node pool a name i.e.,  `kubeip-node-pool=my-node-pool`

```
sed -i "s/pool-kubip/$KUBEIP_SELF_NODEPOOL/g" deploy/kubeip-deployment.yaml
```

Deploy kubeIP by running:

```
kubectl apply -f deploy/.
```

Once you’ve assigned an IP address to a node kubeIP, a label will be created for that node `kubip_assigned` with the value of the IP address (`.` are replaced with `-`):

 `172.31.255.255 ==> 172-31-255-255`


# Deploy & Build From Source

You need Kubernetes version 1.10 or newer. You also need Docker version and kubectl 1.10.x or newer installed on your machine, as well as the Google Cloud SDK. You can install the [Google Cloud SDK](https://cloud.google.com/sdk) (which also installs kubectl).


**Clone Git Repository**

Make sure your $GOPATH is [configured](https://github.com/golang/go/wiki/SettingGOPATH). You'll need to clone this repository to your `$GOPATH/src` folder. 

```
mkdir -p $GOPATH/src/doitintl/kubeip
git clone https://github.com/doitintl/kubeip.git $GOPATH/src/doitintl/kubeip
cd $GOPATH/src/doitintl/kubeip
```

**Set Environment Variables**

Replace **us-central1** with the region where your GKE cluster resides and **kubeip-cluster** with your real GKE cluster name

```
export GCP_REGION=us-central1
export GCP_ZONE=us-central1-b
export GKE_CLUSTER_NAME=kubeip-cluster
export PROJECT_ID=$(gcloud config list --format 'value(core.project)')
```

**Develop kubeIP Locally**

Compile the kubeIP binary and run tests

```
make
```

**Build kubeIP's Container Image**


Compile the kubeIP binary and build the Docker image as following:

```
make image
```

Tag the image using:

```
docker tag kubeip gcr.io/$PROJECT_ID/kubeip
```

Finally, push the image to Google Container Registry with:

```
docker push gcr.io/$PROJECT_ID/kubeip
```

**Create IAM Service Account and obtain the Key in JSON format**

Create a Service Account with this command:

```
gcloud iam service-accounts create kubeip-service-account --display-name "kubeIP"
```

Create and attach the custom kubeIP role to the service account by running the following commands:

```
gcloud iam roles create kubeip --project $PROJECT_ID --file roles.yaml

gcloud projects add-iam-policy-binding $PROJECT_ID --member serviceAccount:kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com --role projects/$PROJECT_ID/roles/kubeip
```

Generate the Key using the following command:

```
gcloud iam service-accounts keys create key.json \
  --iam-account kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com
```

**Create Kubernetes Secret**

Get your GKE cluster credentaials with (replace *cluster_name* with your real GKE cluster name):

```
gcloud container clusters get-credentials $GKE_CLUSTER_NAME \
    --region $GCP_ZONE \
    --project $PROJECT_ID
```

Create a Kubernetes secret by running:

```
kubectl create secret generic kubeip-key --from-file=key.json -n kube-system
```

**We need to get RBAC permissions first with**
```
kubectl create clusterrolebinding cluster-admin-binding \
    --clusterrole cluster-admin --user `gcloud config list --format 'value(core.account)'`
```

**Create static reserved IP addresses:**

Create as many static IP addresses for the number of nodes in your GKE cluster (this example creates 10 addresses) so you will have enough addresses when your cluster scales up (automatically or manually):

```
for i in {1..10}; do gcloud compute addresses create kubeip-ip$i --project=$PROJECT_ID --region=$GCP_REGION; done
```

Add labels to reserved IP addresses. A common practice is to assign a unique value per cluster. You can use your cluster name for example:

```
for i in {1..10}; do gcloud beta compute addresses update kubeip-ip$i --update-labels kubeip=$GKE_CLUSTER_NAME --region $GCP_REGION; done
```

Adjust the deploy/kubeip-configmap.yaml with your GKE cluster name (replace the GKE-cluster-name with your real GKE cluster name):

```
sed -i "s/reserved/$GKE_CLUSTER_NAME/g" deploy/kubeip-configmap.yaml
```

Adjust the `deploy/kubeip-deployment.yaml` to reflect your real container image path:

 - Edit the `image` to match your container image path, i.e. `gcr.io/$PROJECT_ID/kubeip`

By default, kubeIP will only manage the nodes in default-pool nodepool. If you'd like kubeIP to manage another node-pool, please update the `KUBEIP_NODEPOOL` setting in `deploy/kubeip-configmap.yaml` file before deploying. You can also update the `KUBEIP_LABELKEY` and `KUBEIP_LABELVALUE` to control which static external IP addresses the kubeIP will look for to assign to your nodes. 

The `KUBEIP_FORCEASSIGNMENT` (which defaults to true) will check on startup and every five minutes if there are nodes in the node-pool that are not assigned to a reserved address. If such nodes are found, then kubeIP will assign a reserved address (if one is available to them):

Deploy kubeIP by running

```
kubectl apply -f deploy/.
```

References:

 - Event listening code was take from [kubewatch](https://github.com/bitnami-labs/kubewatch/)
