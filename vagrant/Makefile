
SHELL := /bin/bash

CWD := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

ANSIBLE_FLAGS ?= -vv

.PHONY: all
all: destroy up ansible-k8s ansible-credentials ansible-satellite ansible-scripts

# initialize VMs
.PHONY: up
up:
	vagrant up --parallel

# destroy destroys a cluster
.PHONY: destroy
destroy:
	vagrant destroy --force 

# status displays VMs status
.PHONY: status
status:
	vagrant status

# initialize ansible ssh config and inventory
.PHONY: ansible-inventory
ansible-inventory:
	vagrant ssh-config | tee '$(CWD)/ansible/ssh-config'
	machines=($$(vagrant --machine-readable status | cut -d ',' -f 2 | sort -u)) && \
	( \
		echo '[local]' && \
		echo 'localhost ansible_connection=local' && \
		echo '[nodes]' && \
		for (( i=0 ; i<$${#machines[@]}; i++)); do \
			echo "$${machines[i]}" ; \
		done ; \
		echo '[first]' && \
		echo "$${machines[0]}" ; \
	) | tee '$(CWD)/ansible/inventory'

# execute ansible playbooks
.PHONY: ansible-%
ansible-%:  ansible-inventory
	ansible-playbook \
	$(ANSIBLE_FLAGS) \
	--ssh-extra-args='-F $(CWD)/ansible/ssh-config' \
	-i '$(CWD)/ansible/inventory' \
	$(CWD)/ansible/$*.yaml

# ssh connects to a local VM node via SSH
.PHONY: ssh0
ssh0:
	vagrant ssh n1

# ssh connects to a local VM node via SSH
.PHONY: ssh1
ssh1:
	vagrant ssh n2

# ssh connects to a local VM node via SSH
.PHONY: ssh2
ssh2:
	vagrant ssh n3