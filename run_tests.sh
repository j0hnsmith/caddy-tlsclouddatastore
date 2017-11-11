#! /bin/bash


# run the emulator
gcloud beta emulators datastore start > /dev/null 2>&1 &
emulator_pid=$!
$(gcloud beta emulators datastore env-init)
echo "If tests don't run, set DATASTORE_EMULATOR_HOST=localhost:{emulator_port}"


# run the tests
go test . -v

# killing $emulator_pid doesn't always propagate
ps -ef | grep gcloud | grep emulator | awk '{ print $2 }' | xargs kill -2
