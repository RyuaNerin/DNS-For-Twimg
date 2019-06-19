# Domain Name Server For Twimg

- 이 프로젝트는 아직 테스트중입니다.

1. 최근 트위터 이미지 로딩이 느린 현상이 지속되고 있습니다.

2. 이 이미지 로딩 현상이 느린 이유는 현재 한국에서 연결된 `pbs.twimg.com` 의 서버중 한국에서 연결되는 서버의 상태가 좋지 않다고 예상됩니다.

3. 이 DNS 는 한국에서 `pbs.twimg.com` 의 cdn 으로 알려진 서버를 검사하여, 최적의 cdn 을 지정해줍니다.


- 이 라이브러리는 GeoLite2 를 사용중입니다.
  - `GeoLite2-City_20190611`

- 이 프로젝트의 DNS 서버 구현부분은 [kenshinx/godns](https://github.com/kenshinx/godns) 를 참조하여 만들었습니다.

- 이 프로젝트는 [GNU GENERAL PUBLIC LICENSE v3.0](LISENCE)
