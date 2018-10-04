
# gossip

gossip extends the hashicorp/memberlist library to provide additional features
particularly the transport layer.

## Extensions

- Allows running multiple gossip pools on a single port.
- Exposes underlying TCP/UDP transport to run apps on the same port.
