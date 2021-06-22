# AirSENCE data synchronization service for V4.0
<pre>
#######################################################################
#    ___     _            _____    ______    _   __   ______    ______#
#   /   |   (_)   _____  / ___/   / ____/   / | / /  / ____/   / ____/#
#  / /| |  / /   / ___/  \__ \   / __/     /  |/ /  / /       / __/   #
# / ___ | / /   / /     ___/ /  / /___    / /|  /  / /___    / /___   #
#/_/  |_|/_/   /_/     /____/  /_____/   /_/ |_/   \____/   /_____/   #
#              BREATHE SAFE            BREATHE EASY                   #
#######################################################################
</pre>

### AirSENCE data synchronization service has following handlers:
- Receiving Pollutant/Raw data from airsence serivce and them to remote server
- Save Pollutant/Raw data from airsence serivce and save them to local database 
- Resending not successfully sent Pollutant/Raw data in the database to remote server 
- Accepting request of resending Pollutant/Raw within a range of time in the database to remote server


This service has two MQTT clients, local MQTT client and remote MQTT client. Local MQTT client is connected to local MQTT broker and listens to following topic(by default):
- PollutantTopic="airsence/[CUSTOM TAG]/[DEVICE ID]/pollutant"
- RawTopic="airsence/[CUSTOM TAG]/[DEVICE ID]/raw"
- ResendPollutantTopic="airsence/[CUSTOM TAG]/[DEVICE ID]/resendpollutant"
- ResendRawTopic="airsence/[CUSTOM TAG]/[DEVICE ID]/resendraw"

The PollutantTopic and RawTopic should be the same as the airsence service.

Remote MQTT client is connected to remote MQTT broker and listens to following topic(by default):
- ResendPollutantTopic="airsence/[CUSTOM TAG]/[DEVICE ID]/resendpollutant"
- ResendRawTopic="airsence/[CUSTOM TAG]/[DEVICE ID]/resendraw"

Local MQTT client and remote MQTT client are using the same ResendPollutantTopic/ResendRawTopic.

The topic can be changed in the config file. User config file content will overwrite the default config file content when the software read both of them.

### Help Info
The service software has help information. By running
```shell
# datasync -h
```
or
```shell
# datasync --help
``` 
to see the help information.

### Software version
By running
```shell
# datasync -v
```
or
```shell
# datasync --version
```
the current version and build data of the software will be shown


### Config File
In order to run the service, it needs one config file:
- config.tomlz

*config.tomlz* is a zip file, which is encrypted and contains a default config toml file in it.


To run the service software, using
```shell
# datasync -d config.tomlz -c airsenceUser.toml
```
or
```shell
# datasync --default config.tomlz --config airsenceUser.toml
```
