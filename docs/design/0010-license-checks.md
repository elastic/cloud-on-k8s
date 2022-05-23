# 10. License checks

* Status: proposed
* Date: 2019-04-07

Technical Story: tbd

## Context and Problem Statement

We want to control behaviour in the operators based on the current
Enterprise license applied to the operator. An example for conditional 
behaviour are any commercial features available to licensed operators only. 

## Decision Drivers 

* we currently support multiple deployment options for the operator: single namespace and multi-namespace.
* for the activation of commercial features we need a minimal amount of protection against tampering.
* we consider admission controllers as optional means of verification and don't want to rely on them for functionality that does not exist in other places as well.
* we regard licenses as somewhat sensitive data that should not be shared freely across all namespaces and controllers.
  

## Considered Options

1. Configure license statically at operator startup (for example through shared secret bound in all instances) or with a simple flag for trial mode
2. Configure license dynamically and use admission control to label resources (licensed-until timestamp) 
3. Create a shared secret (`ControllerLicense`) in every relevant namespace with tamper proof evidence of the current Enterprise license 

## Decision Outcome

Option 3.


### Additional design details

Create a new CRD called `ControllerLicense` that will be created
by the (global) license controller. It CAN use the same signer we use
for the Enterprise licenses but a different installation specific
private/public key pair.

The public key CAN be shared with the namespace operator at
installation time. The individual controllers that are interested in
license checks will then verify any `ControllerLicenses` appearing in
their namespace using that key and MUST toggle commercial features only if a valid license 
is present.

The license controller MUST create controller licenses only when either a valid 
Enterprise license or a valid Enterprise trial license is present in the system. It CAN
issue controller licenses with shorter lifetimes than the Enterprise license and 
auto-extend them as needed to limit the impact of accidental license leaks. But license leaks 
are currently understood to be much less a concern than cluster licenses leaks as controller licenses have no validity 
outside of the operator installation that has created them.  


```
+--------------------------------+   +----------------------------------+
|                                |   |                                  |
|  elastic-system                |   |       foo-system                 |
|                                |   |                                  |
|                                |   |                                  |
|                                |   |                                  |
|             +------------+     |   |        +-------------+           |
|             |            |     |   |        |             |           |
|             |  license   |     |   |        |    foo      |           |
|             |  ctrl      |     |   |        |    ctrl     |           |
|             |            |     |   |        |             |           |
|             +-+-----+--+-+     |   |        ++-----------++           |
|               ^     |  |       |   |         ^           ^            |
|               |     |  |       |   |         |           |            |
|               |     |  |       |   |         |           |            |
|   +-----------+-+   |  |       |   |   +-----+------+  +-+---------+  |
|   |             |   |  |       |   |   |            |  |           |  |
|   |  Enterprise |   |  |       |   |   | Controller |  |  PubKey   |  |
|   |  License    |   |  +-------------->+ License    |  |           |  |
|   |             |   |  creates |   |   |            |  |           |  |
|   +-------------+   |          |   |   +------------+  +------+----+  |
|                     |          |   |                          ^       |
|                     |          |   |                          |       |
|                     |          |   |   creates                |       |
|                     +-----------------------------------------+       |
|                                |   |                                  |
+--------------------------------+   +----------------------------------+
```

Due to restrictions in controller runtime the license +
secret would need to be deployed into the managed namespace not into
the control plane namespace. Unless of course we run everything in one
namespace anyway or we implement a custom client
that has access to the control plane namespace of the namespace
operator (the latter is the underlying assumption for the license controller graph).

### Positive Consequences 

* Does not give every namespace operator access to the full Enterprise license.
* Allows to issue short term controller licenses to limit impact of exposure.
* Does allow relative protection against tampering.


### Negative Consequences 

* Effort and complexity of the implementation.
* The key/pair to verify the controller license can be manipulated. The global license
  controller can to some extend counteract that by deleting/recreating the correct license 
  and public key resources.
  

## Pros and Cons of the other Options 

### Statically configured license



* Good, because it simplifies the decision for the trial vs. basic decision on cluster creation (but: we might not need that at all if we start with basic by default).
* Good, because it is independent of the deployment scenario chosen for the operator.
* Bad, because it does not solve the burden of proof for commercial feature enablement.
  We would still need a copy of the actual license in all namespaces that have controllers with commercial features (currently only the global one).
* Bad, because it would require an operator restart to update license information.   

### Dynamically configured license and admission controller

* Good, because it solves the problem of propagating license status from a global operator/namespace to the individual namespace operators. 
* Bad, because it is fairly complex.
* Bad, because it makes the admission controller a central and non-optional component for the operator.
* Bad, because it does not offer a convincing solution to update expiry information or to the problem that we might 
  want to re-label resources if new licenses become available.



## Links 

* builds on top of [ADR-0004](0004-licensing.md)  

