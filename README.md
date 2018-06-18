# kIP (pronounced as 'kɪp')

**Prerequisites**

You need a Kubernetes 1.10 or newer cluster. You will also need Docker and kubectl 1.10.x or newer installed on your machine, as well as the Google Cloud SDK. You can install the Google Cloud SDK (which will also install kubectl) [here](https://cloud.google.com/sdk).

**Build the images**

Install go/dep (Go dependency management tool) using [these instructions](https://github.com/golang/dep) by running `dep ensure`

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

Get your GKE cluster credentaials with (replace *cluster_name* and *us-central1* with your real region):

<pre>
gcloud container clusters get-credentials <b>cluster_name</b> \
--region <b>us-central1</b> \
--project $PROJECT_ID
</pre> 

Create a Kubernetes secret by running:

```
kubectl create secret generic kip-key \
--from-file=key.json
```

**Create static reserved IP addresses:** 

Create as many static IP addresses as number of nodes in your GKE cluster (this example creates 4 addresses):

```
for i in {1..4}; do gcloud compute addresses create kip-ip$i --project=$PROJECT_ID --region=us-central1; done
```

Add labels to reserved IP addresses. A common practice is to assign a unique value per cluster (for example cluster name).

```
for i in {1..4}; do gcloud beta compute addresses update kip-ip$i --update-labels kip=reserved --region us-central1; done
```

Adjust the deploy/kip-configmap.yaml with your GKE cluster name (replace the gke-cluster-name with your real GKE cluster name

<pre>
sed -i 's/reserved/<b>gke-cluster-name</b>/g' deploy/kip-configmap.yaml
</pre>

Deploy kIP by running 

```
kubectl apply -f deploy/.
```

References:

 - Event listing code was take from [kubewatch](https://github.com/bitnami-labs/kubewatch/)
