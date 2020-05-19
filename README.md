# Domain Name Server For Twimg

- 이 프로젝트는 아직 테스트중입니다.

- 이 프로젝트는 개인용으로 개발되었습니다.

1. 최근 트위터 이미지 로딩이 느린 현상이 지속되고 있습니다.

2. 한국에서 연 `pbs.twimg.com` 로 연결되는 서버의 상태가 좋지 않아 발생하는 문제라고 예상합니다.

3. 이 DNS 는 한국에서 `pbs.twimg.com` 의 cdn 으로 알려진 서버를 검사하여, 최적의 cdn 을 지정해줍니다.

- 이 라이브러리는 GeoLite2 를 사용중입니다.
    - `GeoLite2-City_20190611`

- 이 프로젝트의 DNS 서버 구현부분은 [kenshinx/godns](https://github.com/kenshinx/godns) 를 참조하여 만들었습니다.

- 이 프로젝트는 [GNU GENERAL PUBLIC LICENSE v3.0](LISENCE) 라이선스 하에 배포됩니다

## Usage

- **아래 주소는 확정된 사항이 아니며 언제든 바뀔 수 있습니다.**

- DNS 설정에 아래 IP 를 제일 상단으로 입력합니다
    - `141.164.49.73`

- 웹에서 최적의 CDN 정보를 확인할 수 있습니다
    - [https://twimg.ryuar.in/](https://twimg.ryuar.in/)

- 타 앱이나 서비스에서도 본 서버의 자세한 측정 결과를 얻어올 수 있습니다
    - [https://twimg.ryuar.in/json](https://twimg.ryuar.in/json)

## Installation & Running

1. 이 레포지토리를 복사합니다.

    ```shell
    git clone https://github.com/RyuaNerin/DNS-For-Twimg.git
    ```

2. build

    ```shell
    go build
    ```

3. running

    ```shell
    sudo go run
    ```

## TODO

- [ ] CDN 체크 후 상태가 좋지 않으면 트위터에 DNS 설정 방식과 함께 트윗하기

- [ ] 웹 페이지 개선

- [x] Ping 이 정상적으로 작동하지 않음.

- [x] cdn 에서 이미지 받아올 때 checksum 검사

- [x] `video.twimg.com` 대응

- 추가 건의사항은 [여기](https://github.com/RyuaNerin/DNS-For-Twimg/issues) 에서 작성해주시면 감사하겠습니다.
