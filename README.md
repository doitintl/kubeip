# KubeIP

Many applications need to be whitelisted by consumers based on source IP address. As of today, Google Kubernetes Engine doesn't support assigning a static pool of addresses to GKE cluster. kubeIP tries to solve this problem by assigning GKE nodes external IP addresses from a predefined list by continually watching the Kubernetes API for new/removed nodes and applying changes accordingly.

# Deploy kubeIP without building from source

If you just want to use KubeIP (instead of building it from source yourself), please follow instructions in this section. You need a Kubernetes 1.10 or newer cluster. You'll also need the Google Cloud SDK. You can install the Google Cloud SDK (which also installs kubectl) [here](https://cloud.google.com/sdk).

Edit deploy/kubeip-configmap.yaml file:

 - Update the `KUBEIP_LABELVALUE` with your real GKE cluster name
 - Update `KUBEIP_NODEPOOL` to match the name of your GKE node-pool on which kubeIP will operate
 
Set the required environment variables: 
 
 ```
export GCP_REGION=us-central1
export GKE_CLUSTER_NAME=kubeip-cluster
export roles=( "roles/compute.admin" "roles/container.clusterAdmin" "roles/compute.storageAdmin" )
export PROJECT_ID=$(gcloud config list --format 'value(core.project)')
```

**Create IAM Service Account and obtain the Key in JSON format**

Create Service Account with this command: 

```
gcloud iam service-accounts create kubeip-service-account --display-name "kubeIP"
```

Attach required roles to the service account by running the following commands:

```
for role in "${roles[@]}"; do gcloud projects add-iam-policy-binding $PROJECT_ID --member serviceAccount:kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com --role $role;done
```

Generate the Key using the following command:

```
gcloud iam service-accounts keys create key.json \
--iam-account kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com
```
 
**Create Kubernetes Secret**

Get your GKE cluster credentaials with (replace *cluster_name* with your real GKE cluster name):

<pre>
gcloud container clusters get-credentials $GKE_CLUSTER_NAME \
--region $GCP_REGION \
--project $PROJECT_ID
</pre> 

Create a Kubernetes secret by running:

```
kubectl create secret generic kubeip-key --from-file=key.json
```

**Create static reserved IP addresses:** 

Create as many static IP addresses as at least the number of nodes in your GKE cluster (this example creates 10 addresses) so you will have enough addresses when your cluster scales up (manually or automatically):

```
for i in {1..10}; do gcloud compute addresses create kubeip-ip$i --project=$PROJECT_ID --region=$GCP_REGION; done
```

Add labels to reserved IP addresses. A common practice is to assign a unique value per cluster (for example cluster name).

```
for i in {1..10}; do gcloud beta compute addresses update kubeip-ip$i --update-labels kubeip=$GKE_CLUSTER_NAME --region $GCP_REGION; done
```

<pre>
sed -i "s/reserved/$GKE_CLUSTER_NAME/g" deploy/kubeip-configmap.yaml
</pre>

Adjust the deploy/kubeip-deployment.yaml to reflect your real container image path:

Deploy kubeIP by running 

```
kubectl apply -f deploy/.
```

# Build From Source

You need a Kubernetes 1.10 or newer cluster. You also need Docker and kubectl 1.10.x or newer installed on your machine, as well as the Google Cloud SDK. You can install the Google Cloud SDK (which also installs kubectl) [here](https://cloud.google.com/sdk).


**Clone Git Repository**

Make sure your $GOPATH is [configured](https://github.com/golang/go/wiki/SettingGOPATH). You'll need to clone this repository to your `$GOPATH/src` folder. 

```
git clone https://github.com/doitintl/kubeIP.git $GOPATH/src/kubeip
cd $GOPATH/src/kubeip 
```

**Set Environment Variables**

Replace **us-central1** with the region where your GKE cluster resides and **kubeip-cluster** with your real GKE cluster name

```
export GCP_REGION=us-central1
export GKE_CLUSTER_NAME=kubeip-cluster
export roles=( "roles/compute.admin" "roles/container.clusterAdmin" "roles/compute.storageAdmin" )
export PROJECT_ID=$(gcloud config list --format 'value(core.project)')
```

**Build kubeIP's container image**

Install go/dep (Go dependency management tool) using [these instructions](https://github.com/golang/dep) and then run

```
dep ensure
```

Compile the kubeIP by running: 

```
make builder-image
```

Build the Docker image with compiled version of kubeIP as following:

```
make binary-image
```

Tag the image using: 

```
docker tag  kubeip gcr.io/$PROJECT_ID/kubeip
```

Finally, push the image to Google Container Registry with: 

```
docker push gcr.io/$PROJECT_ID/kubeip
```

**Create IAM Service Account and obtain the Key in JSON format**

Create Service Account with this command: 

```
gcloud iam service-accounts create kubeip-service-account --display-name "kubeIP"
```

Attach required roles to the service account by running the following commands:

```
for role in "${roles[@]}"; do gcloud projects add-iam-policy-binding $PROJECT_ID --member serviceAccount:kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com --role $role;done
```

Generate the Key using the following command:

```
gcloud iam service-accounts keys create key.json \
--iam-account kubeip-service-account@$PROJECT_ID.iam.gserviceaccount.com
```
 
**Create Kubernetes Secret**

Get your GKE cluster credentaials with (replace *cluster_name* with your real GKE cluster name):

<pre>
gcloud container clusters get-credentials $GKE_CLUSTER_NAME \
--region $GCP_REGION \
--project $PROJECT_ID
</pre> 

Create a Kubernetes secret by running:

```
kubectl create secret generic kubeip-key --from-file=key.json
```

**Create static reserved IP addresses:** 

Create as many static IP addresses as at least the number of nodes in your GKE cluster (this example creates 10 addresses) so you will have enough addresses when your cluster scales up (manually or automatically):

```
for i in {1..10}; do gcloud compute addresses create kubeip-ip$i --project=$PROJECT_ID --region=$GCP_REGION; done
```

Add labels to reserved IP addresses. A common practice is to assign a unique value per cluster (for example cluster name).

```
for i in {1..10}; do gcloud beta compute addresses update kubeip-ip$i --update-labels kubeip=$GKE_CLUSTER_NAME --region $GCP_REGION; done
```

Adjust the deploy/kubeip-configmap.yaml with your GKE cluster name (replace the gke-cluster-name with your real GKE cluster name

<pre>
sed -i "s/reserved/$GKE_CLUSTER_NAME/g" deploy/kubeip-configmap.yaml
</pre>

Adjust the deploy/kubeip-deployment.yaml to reflect your real container image path:

<pre>
sed -i "s/my-project/$PROJECT_ID/g" deploy/kubeip-deployment.yaml
</pre>

By default, kubeIP will only manage the nodes in default-pool nodepool. If you'd like kubeIP to manage another nood-pool, please update the `KUBEIP_NODEPOOL` setting in `deploy/kubeip-configmap.yaml` file before deploying. You can also update the `KUBEIP_LABELKEY` and `KUBEIP_LABELVALUE` to control which static external IP addresses the kubeIP will look for to assign to your nodes.

Deploy kubeIP by running 

```
kubectl apply -f deploy/.
```

References:

 - Event listening code was take from [kubewatch](https://github.com/bitnami-labs/kubewatch/)
