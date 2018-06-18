# kIP (pronounced as 'kɪp')

**Prerequisites**

You need a Kubernetes 1.10 or newer cluster. You will also need Docker and kubectl 1.10.x or newer installed on your machine, as well as the Google Cloud SDK. You can install the Google Cloud SDK (which will also install kubectl) [here](https://cloud.google.com/sdk).

**Build the images**

Install go/dep (Go dependency management tool) using [these instructions](https://github.com/golang/dep)

 - Run `dep ensure`

Build and push the image 
 - Run `gcloud config set project my_project` and replace the my_project with real project_id of your Google Cloud project
 - Run `export PROJECT_ID=$(gcloud config list --format 'value(core.project)')`
 - `make builder-image`
 - `make binary-image`
 - `docker tag  kip gcr.io/$PROJECT_ID/kip`
 - `gcloud docker -- push gcr.io/$PROJECT_ID/kip`

**Create a service account with permission the following roles**

 - Kubernetes Engine Cluster Admin
 - Storage Admin
 - Compute Admin
 
**Download the key file in JSON format:**

Create secret by running `kubectl create secret generic kip-key --from-file=key.json=filename`

**Create static reserved IP addresses:** 

By default **kip** looks for label "kip" with a value "reserved". You can override this by setting `KIP_LABEl_KEY` and `KIP_LABEl_VALUE` in kip-configmap.yaml. 

A common practice is to assign a unique value per cluster (for example cluster name)

Deploy by running `kubctl apply -f deploy/.`
