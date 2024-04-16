pipeline {
  agent {
    kubernetes {
      label "kubewatch-${env.BRANCH_NAME}-${UUID.randomUUID()}"
      yaml """
apiVersion: v1
kind: Pod
spec:
  containers:
    - name: dind
      image: sc-mum-armory.platform.internal/devops/dind:v2
      securityContext:
        privileged: true
      env:
      - name: DOCKER_HOST
        value: tcp://localhost:2375
      - name: DOCKER_TLS_CERTDIR
        value: ""
      volumeMounts:
        - name: dind-storage
          mountPath: /var/lib/docker
      readinessProbe:
        tcpSocket:
          port: 2375
        initialDelaySeconds: 30
      livenessProbe:
        tcpSocket:
          port: 2375
        initialDelaySeconds: 30
    - name: builder
      image: sc-mum-armory.platform.internal/devops/builder-image-golang-1.18-armory
      command:
        - sleep
        - infinity
      env:
        - name: DOCKER_HOST
          value: tcp://localhost:2375
        - name: DOCKER_BUILDKIT
          value: "0"
      volumeMounts:
        - name: jenkins-sa
          mountPath: /root/.gcp/
  volumes:
    - name: dind-storage
      emptyDir: {}
    - name: jenkins-sa
      secret:
        secretName: jenkins-sa
"""
    }
  }
  environment{
    clouds="gcp,oci"
    sc_regions="singapore"
    app="kubewatch"
    imagetags="v1.2.3"
  }
  stages {
    stage('docker build') {
      steps {
        container('builder') {
            sh 'armory build'
        }
      }
    }
    stage('push') {
      when {
        anyOf {
                  branch 'main'
                  branch 'custom_k8s_events_for_webhook_int'
                  branch 'docker-test'
        }
      }
      steps {
        container('builder') {
          sh "armory push"
        }
      }
    }
  }
}
