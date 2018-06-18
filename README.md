# kIP (pronounced as 'kɪp')

Many applications need to be whitelisted by consumers based on source IP address. As of today, Google Kubernetes Engine doesn't support assigning a static pool of addresses to GKE cluster. kIP tries to solve this problem with assigning GKE nodes external IP addresses from a predefined list by constantly watching the Kubernetes API for new/removed nodes and applying changes accordingly. 

**Prerequisites**

You need a Kubernetes 1.10 or newer cluster. You will also need Docker and kubectl 1.10.x or newer installed on your machine, as well as the Google Cloud SDK. You can install the Google Cloud SDK (which will also install kubectl) [here](https://cloud.google.com/sdk).

**Clone Git Repository**

Make sure your $GOPATH is [configured](https://github.com/golang/go/wiki/SettingGOPATH). You'll need to clone this repository to your `$GOPATH/src` folder. 

```
git clone https://github.com/doitintl/kIP.git $GOPATH/src/kip
cd $GOPATH/src/kip 
```

**Set Environment Variables**

Replace **us-central1** with the region where your GKE cluster resides and **kip-cluster** with your real GKE cluster name

```
export GCP_REGION=us-central1
export GKE_CLUSTER_NAME=kip-cluster
```

**Build the images**

Install go/dep (Go dependency management tool) using [these instructions](https://github.com/golang/dep) and then run

```
dep ensure
```

Build and push the image:

 - Set project with `gcloud config set project my_project` and replace the my_project with your [project_id](https://cloud.google.com/storage/docs/projects)
 - Run `export PROJECT_ID=$(gcloud config list --format 'value(core.project)')`
 - Compile the kIP with `make builder-image`
 - build the Docker image with compiled version of kIP `make binary-image`
 - Tag the image using `docker tag  kip gcr.io/$PROJECT_ID/kip`
 - Push the image to Google Container Registry with `docker push gcr.io/$PROJECT_ID/kip`

**Create IAM Service Account and obtain the Key in JSON format**

Create Service Account with this command 

```
gcloud iam service-accounts create kip-service-account \
--display-name "kIP"
```

Attach required roles to the service account by running the following commands:

```
gcloud projects add-iam-policy-binding $PROJECT_ID \
--member serviceAccount:kip-service-account@$PROJECT_ID.iam.gserviceaccount.com \
--role roles/compute.admin

gcloud projects add-iam-policy-binding $PROJECT_ID \
--member serviceAccount:kip-service-account@$PROJECT_ID.iam.gserviceaccount.com \
--role roles/container.clusterAdmin

gcloud projects add-iam-policy-binding $PROJECT_ID \
--member serviceAccount:kip-service-account@$PROJECT_ID.iam.gserviceaccount.com \
--role roles/compute.storageAdmin
```

Generate the Key using the following command:

```
gcloud iam service-accounts keys create key.json \
--iam-account kip-service-account@$PROJECT_ID.iam.gserviceaccount.com
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
kubectl create secret generic kip-key \
--from-file=key.json
```

**Create static reserved IP addresses:** 

Create as many static IP addresses as at least the number of nodes in your GKE cluster (this example creates 10 addresses) so you will have enough addresses when your cluster scales up (manually or automatically):

```
for i in {1..10}; do gcloud compute addresses create kip-ip$i --project=$PROJECT_ID --region=$GCP_REGION; done
```

Add labels to reserved IP addresses. A common practice is to assign a unique value per cluster (for example cluster name).

```
for i in {1..10}; do gcloud beta compute addresses update kip-ip$i --update-labels kip=$GKE_CLUSTER_NAME --region $GCP_REGION; done
```

Adjust the deploy/kip-configmap.yaml with your GKE cluster name (replace the gke-cluster-name with your real GKE cluster name

<pre>
sed -i "s/reserved/$GKE_CLUSTER_NAME/g" deploy/kip-configmap.yaml
</pre>

Adjust the deploy/kip-deployment.yaml to reflect your real container image path:

<pre>
sed -i "s/my-project/$PROJECT_ID/g" deploy/kip-deployment.yaml
</pre>

Deploy kIP by running 

```
kubectl apply -f deploy/.
```

References:

 - Event listing code was take from [kubewatch](https://github.com/bitnami-labs/kubewatch/)
