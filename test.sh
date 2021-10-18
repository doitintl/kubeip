#!/usr/bin/env bash -xe
if [ -z "$REGION" ]; then
 echo REGION not defined!
 exit
fi
if [ -z "$CLUSTER" ]; then
 echo CLUSTER not defined!
 exit
fi

NODES=`gcloud container node-pools describe default-pool --cluster $CLUSTER|grep initialNodeCount|awk '{print $2}'`
NEW_NODES=$(($NODES + 1))
gcloud compute addresses create kubeip-test-1 --region $REGION
gcloud beta compute addresses update kubeip-test-1 --update-labels kubeip=reserved  --region us-central1
IP1=`gcloud compute addresses describe kubeip-test-1 --region $REGION|grep address:|awk '{print $2}'`
gcloud compute addresses create kubeip-test-2 --region $REGION
gcloud beta compute addresses update kubeip-test-2 --update-labels kubeip=reserved  --region us-central1
IP2=`gcloud compute addresses describe kubeip-test-2 --region $REGION|grep address:|awk '{print $2}'`
gcloud beta container clusters resize $CLUSTER --node-pool default-pool --size $NEW_NODES --quiet

STATUS1=`gcloud compute addresses describe kubeip-test-1 --region $REGION|grep status|awk '{print $2}'`
STATUS2=`gcloud compute addresses describe kubeip-test-2 --region $REGION|grep status|awk '{print $2}'`
echo 'expecting one IP IN_USE and one RESERVED'
echo 'Results:'
echo $STATUS1 '--' $STATUS2

gcloud beta container clusters resize $CLUSTER --node-pool default-pool --size $NODES --quiet
gcloud compute addresses delete kubeip-test-1 --region $REGION --quiet
gcloud compute addresses delete kubeip-test-2 --region $REGION --quiet
