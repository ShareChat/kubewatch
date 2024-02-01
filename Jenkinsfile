pipeline {
  agent {
    kubernetes {
      label 'kubewatch'
      yaml """
apiVersion: v1
kind: Pod
spec:
  containers:
    - name: dind
      image: sc-mum-armory.platform.internal/devops/dnd:v2
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
    sc_regions="mumbai,singapore"
    moj_regions="us,singapore"
    app="kubewatch"
    buildarg_DEPLOYMENT_ID="feed-service-$GIT_COMMIT"
    buildarg_GITHUB_TOKEN=credentials('github-access')
    imagetag="v1"
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
      environment {
        DOCKER_REPO = "sc-mum-armory.platform.internal/sharechat/kubewatch"
      }
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
