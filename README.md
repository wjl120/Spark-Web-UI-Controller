# Spark Web UI Controller
## Introduce
### Why Spark On Kubernetes?
Benefits of Spark on Kubernetes:
- Containerization  
The benefits of containerization in traditional software engineering apply to big data and Spark too. Containers make your applications more portable, they simplify the packaging of dependencies, they enable repeatable and reliable build workflows. They reduce the overall devops load and allow you to iterate on your code faster.  
The top 3 benefits of using Docker containers for Spark:  
  1、 Build your dependencies once, run everywhere (locally or at scale)  
  2、 Make Spark more reliable and cost-efficient.  
  3、 Speed up your iteration cycle 
- Integration in a rich ecosystem  
Deploying Spark on Kubernetes gives you powerful features for free such as the use of  namespaces and quotas for multitenancy control, and role-based access control (optionally integrated with your cloud provider IAM) for fine-grained security and data access.
- Efficient resource sharing  
On other cluster-managers (YARN, Standalone, Mesos) if you want to reuse the same cluster for concurrent Spark apps (for cost reasons), you'll have to compromise on isolation:  
  1、 Dependency isolation. These apps must use the same global Spark and python version.  
  2、 Performance isolation. If someone else kicks off a big job, my job is likely to run slower.
On the other hand, with dynamic allocation and cluster autoscaling correctly configured, Kubernetes will give you the cost benefits of a shared infrastructure and the full isolation of disjoint container sets. It takes about 10s for Kubernetes to remove an idle Spark executor from one app and allocate this capacity to another app.
AWS Simplify running Apache Spark jobs with Amazon EMR on Amazon EKS:  
https://aws.amazon.com/cn/about-aws/whats-new/2020/12/simplify-running-apache-spark-jobs-amazon-emr-amazon-eks/

### What is Spark Web UI?
Apache Spark provides a suite of web user interfaces (UIs) that you can use to monitor the status and resource consumption of your Spark cluster.

### How to access Spark Web UI?
- Standalone： Access: http://IP:4040
- Cluster mode：Through Spark log server xxxxxx:18088 or yarn UI, and enter the corresponding Spark UI interface.

### Problems accessing Spark Web UI when Spark On EKS
- The Spark Web UI does not have a fixed access address and the address it is internal of EKS and is not directly accessible externally.
- Cannot fix the web UI for each spark task to an external ELB.

## Architecture
![](https://github.com/wjl120/Spark-Web-UI-Controller-/blob/main/Architecture.png)
### Basic logic  
  1、Use client-go reflector list && watch spark driver service and send to workqueue.  
  2、Use client-go reflector list && watch ingressroute.  
  3、Itorate workqueue, when get spark driver svc notification, try to create spark ui and ingressroute.
  
## Compile


## Build Image

## Useage




