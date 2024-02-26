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
      image: sc-mum-armory.platform.internal/devops/dnd:v1
      securityContext:
        privileged: true
      volumeMounts:
        - name: dind-storage
          mountPath: /var/lib/docker
    - name: builder
      image: sc-mum-armory.platform.internal/devops/builder-image-golang-1.19.5-armory
      command:
        - sleep
        - infinity
      env:
        - name: DOCKER_HOST
          value: tcp://localhost:2375
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
    sc_regions="mumbai"
    app="consul-eds"
    imagetags="v1.2.3"
  }
  stages {
    stage('docker login') {
      steps {
        container('builder') {
            sh 'cat /root/.gcp/jenkins-sa.json | docker login -u _json_key --password-stdin https://asia.gcr.io'
        }
      }
    }
    stage('docker build') {
      steps {
        container('builder') {
            sh 'armory build -f Docker-test'
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
