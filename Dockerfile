FROM openjdk:8
COPY ./test.jar /usr
WORKDIR /usr
ENTRYPOINT ["java", "-jar", "./test.jar","echo $JAVA_OPTS"]
