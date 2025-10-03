#!/bin/bash
echo "Script is running!"
echo "Checking if template was generated..."
if [ -f .data/generated/homelab_ssh_config ]; then
    echo "✓ Template was generated successfully before script ran"
else
    echo "✗ Template was NOT generated"
fi
