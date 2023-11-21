<!--
SPDX-License-Identifier: Apache-2.0
-->

# Features added to PFCP-Agent of OMEC UPF


### RegisterTolb Function
UPF on start-up after creation of its PFCPIface (PFCP-Agent), in Run Function do its initialization tasks. After that, it should register itself on both East-LB and West-LB. This job is done by the RegisterTolb function. This function prepares an HTTP POST request with a JSON body that contains the IP of the gateway of the core interface of UPF (found by the getExitLbInt function), the Mac address of the core and access interfaces of UPF (retrieved by the GetMac function), and the host name of UPF. The IP of the gateway is used by the load balancers to find out the incoming request belongs to which UPF, and that UPF is connecting to which of their interfaces; the Mac addresses are used to add static ARP for the routing purposes and also to track the state of UPFs; and finally, the hostname is used when the load balancers, if in any case, want to send any message toward UPFs. The messages will be sent to the service name to the related URL. Which in this case are “http://enterlb:8080/register” for West-LB and “http://exitlb:8080/addrule” for the East-LB. This message will be sent repeatedly if the 201 created response is not received.

### HTTP Server
After the creation of PFCPIface, during the init phase of UPF (in the mustInit function), an HTTP server is created in the normal implementation of UPF by SD-Core. In its handler function (setupConfigHandler), the "/v1/config/network-slices" path is already created. Besides that, I added another path ("/registergw"), which is used to handle the messages from load balancers. In its handler, it expects to receive an HTTP POST request, which contains an IP and a Mac address, which are the IP and Mac addresses of the corresponding interfaces of East-LB and West-LB. As decided, in virtual UPF, the mac addresses of UPFs and load balancers are handled statically, which means the mac addresses of related interfaces of UPFs, West-LB and East-LB, are entered into their ARP cache by static ARP.

If a load balancer sends its IP and mac address, it means that the UPF is successfully registered in it, and the mac addresses of the UPF are added to load balancers. Now it's time to add the mac addresses of East-LB and West-LB in UPF. This handler adds the IP and mac address of received messages from load balancers to its ARP cache.

### PushPFCPInfoNew function
It is called on the reception of the first received HTTP POST request from load balancers. It continuously checks if it has received a message from both East-LB and West-LB. Once it receives both messages and adds their Mac addresses, everything is set between West-LB, UPF, and East-LB. which means the UPF and load balancers are now ready to get configured for handling data plane traffic from UEs. The next step is the registration of UPF on PFCP-LB, which is done by the PushPFCPInfoNew function. This function creates an HTTP POST request and puts the information of the UPF object in it. The most important field of the UPF object is hostname, which is used by PFCP-LB to manage the internal UPFs on the Kubernetes cluster. This message is sent to the http server of PFCP-LB, whose address in this project is “http://UPF-http:8081/”. This message will be sent repeatedly if the 201 created response is not received (which means until PFCP-LB becomes ready to handle this kind of request from UPFs).

### PushPDRInfo function
when a UPF receives a PFCP message from downPFCP-Agent, in the handleSessionEstablishmentRequest or handleSessionModificationRequest functions, it handles the message and saves its session information to its local store using the PutSession function. At the end of the PutSession function, when we are sure that the session is saved successfully on the PFCP agent of UPF, the PushPDRInfo will be called. This function sends the IP address of UE (which can be read from the session) along with the IP of the gateway of the core interface of UPF (which is found by the getExitLbInt function) with an HTTP POST request toward the HTTP servers of East-LB and West-LB using their service names to the related URL. Which in this case are “http://enterlb:8080/addrule” for the West-LB and “http://exitlb:8080/addrule” for the East-LB. The transmission of this message is implemented with the sendToLBer function. In this way, the load balancers know that the traffic from each UE should be sent to which UPF.

* In live session migration, the downPFCP-Agent generates the PFCP messages to configure the destination UPF. In this case, the down-PFCP agent puts a certain value (for example, 123) on the MessagePriority field of the PFCP message. On the other hand, once load balancers receive UE information from UPFs, they instantly create (or update) their forwarding logic to route the traffic of UE toward the correct UPF. And we know that the UPF first receives a PFCP session establishment request, then receives a session modification request, and after processing them, it is ready to accept the traffic from the related UE. So, to support the live session migration, it’s very important that if message priority shows that a session migration is happening, the UPF sends the UE information after processing the PFCP session modification request (not the PFCP session establishment request).

### sendToLBer Function
is responsible for sending HTTP messages toward the provided address. This function, after the transmission, waits for a response, and if it doesn’t get the response during the predefined timeout, it resends the message. And it stops transmitting once it receives a 201 Created response or reaches the maximum number of retries. If the received message is not 201 create, it generates an error.

### getExitLbInt function
Is responsible for finding the IP of the gateway of the core interface of UPF.

### GetMac function
Is responsible for finding the Mac address of the input interface on UPF.

* Since the IP address of the gateway of the core interface of the UPF and the Mac addresses of the core and access interfaces are fixed while the UPF is running, to prevent additional unnecessary processes, the getExitLbInt and GetMac functions are run only once, and the result is saved. Whenever the UPF needs these values, they are read from the saved value.

### Other changes

* To facilitate session management on virtual UPF, it is necessary that all PFCP Agents in virtual UPF have the same SEID (which is generated by the upPFCP-Agent of PFCP-LB) for each PFCP session so that its uniqueness can be guaranteed by the upPFCP-Agent. To support this idea, the UPF in the NewPFCPSession function, which is called in the handleSessionEstablishmentRequest function, when wanting to create a new PFCP session, instead of generating a new LSEID, uses the FSEID of the received message (which is the LSEID of upPFCP-Agent) as its own LSEID.

* On the response of the PFCP messages, there is a field called MessagePriority. In the implementation of SD-Core, the 0 is hard coded as its value for any kind of message. To support the live session migration, it is necessary that the UPF, on creation of every kind of PFCP response message (session establishment, session modification, and session deletion), set the MessagePriority equal to the MessagePriority of the received request (which was sent by downPFCP-Agent) and then send that response.
