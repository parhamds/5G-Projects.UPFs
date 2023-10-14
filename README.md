<!--
SPDX-License-Identifier: Apache-2.0
-->

# UPFs

It basically is the [OMEC UPF](https://github.com/omec-project/upf), that I added some features to its PFCP-Agent container. 


### Changes made to OMEC UPF
  * The UPF registers itself on Access-LB and Core-LB after startup
  * After registering to Access-LB and Core-LB it registers itself to PFCP-LB
  * On every reception  of a PFCP Session Establishment Request, it sends the IP of associated UE to Access-LB and Core-LB
  * If the message priority of received PFCP message is equal to 123, it sends the IP of UE to Access-lb and Core-lb after handling the PFCP-Session Modification Request
  * PFCP port changet to 8806 



### Create docker image



To build all Docker images run:

```
make docker-build
```

To build a selected image use `DOCKER_TARGETS`:

```
DOCKER_TARGETS=pfcpiface make docker-build
```
