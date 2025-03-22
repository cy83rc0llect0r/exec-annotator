package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	log "github.com/sirupsen/logrus"
)

func main() {
	http.HandleFunc("/mutate", validateExec)
	log.Info("Starting webhook server on :8443")
	log.Fatal(http.ListenAndServeTLS(":8443", "/etc/webhook/certs/tls.crt", "/etc/webhook/certs/tls.key", nil))
}

func validateExec(w http.ResponseWriter, r *http.Request) {
	var admissionReview v1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&admissionReview); err != nil {
		log.Errorf("Failed to decode admission review: %v", err)
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	req := admissionReview.Request
	if req == nil {
		log.Warn("No request found in admission review")
		sendResponse(w, admissionReview, true)
		return
	}

	if req.Operation != v1.Connect || req.SubResource != "exec" {
		log.Infof("Skipping non-exec request: operation=%s, subresource=%s", req.Operation, req.SubResource)
		sendResponse(w, admissionReview, true)
		return
	}

	err := annotatePod(req.Namespace, req.Name, req.UserInfo.Username)
	if err != nil {
		log.Errorf("Failed to annotate pod: %v", err)
		http.Error(w, "Failed to annotate pod", http.StatusInternalServerError)
		return
	}

	sendResponse(w, admissionReview, true)
}

func annotatePod(namespace, podName, username string) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations["exec-user"] = username
	pod.Annotations["exec-time"] = time.Now().UTC().Format(time.RFC3339)

	_, err = clientset.CoreV1().Pods(namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	log.Infof("Added annotations to pod %s/%s: exec-user=%s, exec-time=%s", namespace, podName, username, pod.Annotations["exec-time"])
	return nil
}

func sendResponse(w http.ResponseWriter, review v1.AdmissionReview, allowed bool) {
	resp := v1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: allowed,
	}
	if !allowed {
		resp.Result = &metav1.Status{Message: "Failed to process exec request"}
	}

	review.Response = &resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}
