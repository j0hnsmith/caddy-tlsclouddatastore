#! /bin/bash

# run the emulator
gcloud beta emulators datastore start > /dev/null 2>&1 &
emulator_pid=$!
$(gcloud beta emulators datastore env-init)


# run the tests
go test .

# killing $emulator_pid doesn't always propagate
ps -ef | grep gcloud | grep emulator | awk '{ print $2 }' | xargs kill -2
