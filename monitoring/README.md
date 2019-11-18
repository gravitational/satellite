# Health Checks

*   aws                 -   Verifies that an AWS IAM profile is assigned to this node.
*   boot_config_linux   -   Verifies kernel configuration options on host.
*   cgroup              -   Verifies cgroups have been mounted.
*   dns                 -   Verifies DNS servers are responding.
*   docker              -   Verifies docker devicemapper disk does not exceed safe utilization limit.
*   etcd                -   Verifies