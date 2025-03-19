#!/bin/bash

# Build the docbase-cli binary
echo "Building docbase-cli..."
go build -o docbase .

# Install the binary to /usr/local/bin
echo "Installing to /usr/local/bin..."
sudo mv docbase /usr/local/bin/

echo "Done! You can now run 'docbase' from anywhere."