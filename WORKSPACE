load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    urls = [
        "https://storage.googleapis.com/bazel-mirror/github.com/bazelbuild/rules_go/releases/download/0.18.6/rules_go-0.18.6.tar.gz",
        "https://github.com/bazelbuild/rules_go/releases/download/0.18.6/rules_go-0.18.6.tar.gz",
    ],
    sha256 = "f04d2373bcaf8aa09bccb08a98a57e721306c8f6043a2a0ee610fd6853dcde3d",
)

http_archive(
    name = "bazel_gazelle",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.17.0/bazel-gazelle-0.17.0.tar.gz"],
    sha256 = "3c681998538231a2d24d0c07ed5a7658cb72bfb5fd4bf9911157c0e9ac6a2687",
)

load("@io_bazel_rules_go//go:deps.bzl", "go_rules_dependencies", "go_register_toolchains")

go_rules_dependencies()

go_register_toolchains()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

go_repository(
    name = "com_github_appscode_jsonpatch",
    commit = "7c0e3b262f30165a8ec3d0b4c6059fd92703bfb2",
    importpath = "github.com/appscode/jsonpatch",
)

go_repository(
    name = "com_github_beorn7_perks",
    commit = "3a771d992973f24aa725d07868b467d1ddfceafb",
    importpath = "github.com/beorn7/perks",
)

go_repository(
    name = "com_github_davecgh_go_spew",
    commit = "8991bc29aa16c548c550c7ff78260e27b9ab7c73",
    importpath = "github.com/davecgh/go-spew",
)

go_repository(
    name = "com_github_docker_spdystream",
    commit = "6480d4af844c189cf5dd913db24ddd339d3a4f85",
    importpath = "github.com/docker/spdystream",
)

go_repository(
    name = "com_github_elastic_go_ucfg",
    commit = "0539807037ce820e147797f051ff32b05f4f9288",
    importpath = "github.com/elastic/go-ucfg",
)

go_repository(
    name = "com_github_emicklei_go_restful",
    commit = "3eb9738c1697594ea6e71a7156a9bb32ed216cf0",
    importpath = "github.com/emicklei/go-restful",
)

go_repository(
    name = "com_github_evanphx_json_patch",
    commit = "72bf35d0ff611848c1dc9df0f976c81192392fa5",
    importpath = "github.com/evanphx/json-patch",
)

go_repository(
    name = "com_github_fsnotify_fsnotify",
    commit = "c2828203cd70a50dcccfb2761f8b1f8ceef9a8e9",
    importpath = "github.com/fsnotify/fsnotify",
)

go_repository(
    name = "com_github_ghodss_yaml",
    commit = "0ca9ea5df5451ffdf184b4428c902747c2c11cd7",
    importpath = "github.com/ghodss/yaml",
)

go_repository(
    name = "com_github_go_logr_logr",
    commit = "9fb12b3b21c5415d16ac18dc5cd42c1cfdd40c4e",
    importpath = "github.com/go-logr/logr",
)

go_repository(
    name = "com_github_go_logr_zapr",
    commit = "7536572e8d55209135cd5e7ccf7fce43dca217ab",
    importpath = "github.com/go-logr/zapr",
)

go_repository(
    name = "com_github_go_test_deep",
    commit = "6592d9cc0a499ad2d5f574fde80a2b5c5cc3b4f5",
    importpath = "github.com/go-test/deep",
)

go_repository(
    name = "com_github_gobuffalo_envy",
    commit = "801d7253ade1f895f74596b9a96147ed2d3b087e",
    importpath = "github.com/gobuffalo/envy",
)

go_repository(
    name = "com_github_gogo_protobuf",
    commit = "4cbf7e384e768b4e01799441fdf2a706a5635ae7",
    importpath = "github.com/gogo/protobuf",
)

go_repository(
    name = "com_github_golang_groupcache",
    commit = "c65c006176ff7ff98bb916961c7abbc6b0afc0aa",
    importpath = "github.com/golang/groupcache",
)

go_repository(
    name = "com_github_golang_protobuf",
    commit = "aa810b61a9c79d51363740d207bb46cf8e620ed5",
    importpath = "github.com/golang/protobuf",
)

go_repository(
    name = "com_github_google_btree",
    commit = "4030bb1f1f0c35b30ca7009e9ebd06849dd45306",
    importpath = "github.com/google/btree",
)

go_repository(
    name = "com_github_google_gofuzz",
    commit = "24818f796faf91cd76ec7bddd72458fbced7a6c1",
    importpath = "github.com/google/gofuzz",
)

go_repository(
    name = "com_github_google_uuid",
    commit = "9b3b1e0f5f99ae461456d768e7d301a7acdaa2d8",
    importpath = "github.com/google/uuid",
)

go_repository(
    name = "com_github_googleapis_gnostic",
    commit = "7c663266750e7d82587642f65e60bc4083f1f84e",
    importpath = "github.com/googleapis/gnostic",
)

go_repository(
    name = "com_github_gregjones_httpcache",
    commit = "c63ab54fda8f77302f8d414e19933f2b6026a089",
    importpath = "github.com/gregjones/httpcache",
)

go_repository(
    name = "com_github_hashicorp_go_reap",
    commit = "bf58d8a43e7b6bf026d99d6295c4de703b67b374",
    importpath = "github.com/hashicorp/go-reap",
)

go_repository(
    name = "com_github_hashicorp_golang_lru",
    commit = "20f1fb78b0740ba8c3cb143a61e86ba5c8669768",
    importpath = "github.com/hashicorp/golang-lru",
)

go_repository(
    name = "com_github_hashicorp_hcl",
    commit = "8cb6e5b959231cc1119e43259c4a608f9c51a241",
    importpath = "github.com/hashicorp/hcl",
)

go_repository(
    name = "com_github_hpcloud_tail",
    commit = "a30252cb686a21eb2d0b98132633053ec2f7f1e5",
    importpath = "github.com/hpcloud/tail",
)

go_repository(
    name = "com_github_imdario_mergo",
    commit = "9f23e2d6bd2a77f959b2bf6acdbefd708a83a4a4",
    importpath = "github.com/imdario/mergo",
)

go_repository(
    name = "com_github_inconshreveable_mousetrap",
    commit = "76626ae9c91c4f2a10f34cad8ce83ea42c93bb75",
    importpath = "github.com/inconshreveable/mousetrap",
)

go_repository(
    name = "com_github_joho_godotenv",
    commit = "23d116af351c84513e1946b527c88823e476be13",
    importpath = "github.com/joho/godotenv",
)

go_repository(
    name = "com_github_json_iterator_go",
    commit = "1624edc4454b8682399def8740d46db5e4362ba4",
    importpath = "github.com/json-iterator/go",
)

go_repository(
    name = "com_github_magiconair_properties",
    commit = "c2353362d570a7bfa228149c62842019201cfb71",
    importpath = "github.com/magiconair/properties",
)

go_repository(
    name = "com_github_markbates_inflect",
    commit = "24b83195037b3bc61fcda2d28b7b0518bce293b6",
    importpath = "github.com/markbates/inflect",
)

go_repository(
    name = "com_github_matttproud_golang_protobuf_extensions",
    commit = "c12348ce28de40eed0136aa2b644d0ee0650e56c",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
)

go_repository(
    name = "com_github_mitchellh_mapstructure",
    commit = "3536a929edddb9a5b34bd6861dc4a9647cb459fe",
    importpath = "github.com/mitchellh/mapstructure",
)

go_repository(
    name = "com_github_modern_go_concurrent",
    commit = "bacd9c7ef1dd9b15be4a9909b8ac7a4e313eec94",
    importpath = "github.com/modern-go/concurrent",
)

go_repository(
    name = "com_github_modern_go_reflect2",
    commit = "4b7aa43c6742a2c18fdef89dd197aaae7dac7ccd",
    importpath = "github.com/modern-go/reflect2",
)

go_repository(
    name = "com_github_onsi_ginkgo",
    commit = "2e1be8f7d90e9d3e3e58b0ce470f2f14d075406f",
    importpath = "github.com/onsi/ginkgo",
)

go_repository(
    name = "com_github_onsi_gomega",
    commit = "65fb64232476ad9046e57c26cd0bff3d3a8dc6cd",
    importpath = "github.com/onsi/gomega",
)

go_repository(
    name = "com_github_pborman_uuid",
    commit = "adf5a7427709b9deb95d29d3fa8a2bf9cfd388f1",
    importpath = "github.com/pborman/uuid",
)

go_repository(
    name = "com_github_pelletier_go_toml",
    commit = "c01d1270ff3e442a8a57cddc1c92dc1138598194",
    importpath = "github.com/pelletier/go-toml",
)

go_repository(
    name = "com_github_petar_gollrb",
    commit = "53be0d36a84c2a886ca057d34b6aa4468df9ccb4",
    importpath = "github.com/petar/GoLLRB",
)

go_repository(
    name = "com_github_peterbourgon_diskv",
    commit = "5f041e8faa004a95c88a202771f4cc3e991971e6",
    importpath = "github.com/peterbourgon/diskv",
)

go_repository(
    name = "com_github_pkg_errors",
    commit = "645ef00459ed84a119197bfb8d8205042c6df63d",
    importpath = "github.com/pkg/errors",
)

go_repository(
    name = "com_github_pmezard_go_difflib",
    commit = "792786c7400a136282c1664665ae0a8db921c6c2",
    importpath = "github.com/pmezard/go-difflib",
)

go_repository(
    name = "com_github_prometheus_client_golang",
    commit = "505eaef017263e299324067d40ca2c48f6a2cf50",
    importpath = "github.com/prometheus/client_golang",
)

go_repository(
    name = "com_github_prometheus_client_model",
    commit = "5c3871d89910bfb32f5fcab2aa4b9ec68e65a99f",
    importpath = "github.com/prometheus/client_model",
)

go_repository(
    name = "com_github_prometheus_common",
    commit = "4724e9255275ce38f7179b2478abeae4e28c904f",
    importpath = "github.com/prometheus/common",
)

go_repository(
    name = "com_github_prometheus_procfs",
    commit = "1dc9a6cbc91aacc3e8b2d63db4d2e957a5394ac4",
    importpath = "github.com/prometheus/procfs",
)

go_repository(
    name = "com_github_rogpeppe_go_internal",
    commit = "d87f08a7d80821c797ffc8eb8f4e01675f378736",
    importpath = "github.com/rogpeppe/go-internal",
)

go_repository(
    name = "com_github_spf13_afero",
    commit = "d40851caa0d747393da1ffb28f7f9d8b4eeffebd",
    importpath = "github.com/spf13/afero",
)

go_repository(
    name = "com_github_spf13_cast",
    commit = "8c9545af88b134710ab1cd196795e7f2388358d7",
    importpath = "github.com/spf13/cast",
)

go_repository(
    name = "com_github_spf13_cobra",
    commit = "ef82de70bb3f60c65fb8eebacbb2d122ef517385",
    importpath = "github.com/spf13/cobra",
)

go_repository(
    name = "com_github_spf13_jwalterweatherman",
    commit = "4a4406e478ca629068e7768fc33f3f044173c0a6",
    importpath = "github.com/spf13/jwalterweatherman",
)

go_repository(
    name = "com_github_spf13_pflag",
    commit = "298182f68c66c05229eb03ac171abe6e309ee79a",
    importpath = "github.com/spf13/pflag",
)

go_repository(
    name = "com_github_spf13_viper",
    commit = "6d33b5a963d922d182c91e8a1c88d81fd150cfd4",
    importpath = "github.com/spf13/viper",
)

go_repository(
    name = "com_github_stretchr_testify",
    commit = "ffdc059bfe9ce6a4e144ba849dbedead332c6053",
    importpath = "github.com/stretchr/testify",
)

go_repository(
    name = "com_google_cloud_go",
    commit = "0ebda48a7f143b1cce9eb37a8c1106ac762a3430",
    importpath = "cloud.google.com/go",
)

go_repository(
    name = "in_gopkg_fsnotify_v1",
    commit = "c2828203cd70a50dcccfb2761f8b1f8ceef9a8e9",
    importpath = "gopkg.in/fsnotify.v1",
    remote = "https://github.com/fsnotify/fsnotify.git",
    vcs = "git",
)

go_repository(
    name = "in_gopkg_inf_v0",
    commit = "d2d2541c53f18d2a059457998ce2876cc8e67cbf",
    importpath = "gopkg.in/inf.v0",
)

go_repository(
    name = "in_gopkg_tomb_v1",
    commit = "dd632973f1e7218eb1089048e0798ec9ae7dceb8",
    importpath = "gopkg.in/tomb.v1",
)

go_repository(
    name = "in_gopkg_yaml_v2",
    commit = "51d6538a90f86fe93ac480b35f37b2be17fef232",
    importpath = "gopkg.in/yaml.v2",
)

go_repository(
    name = "io_k8s_api",
    commit = "05914d821849570fba9eacfb29466f2d8d3cd229",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/api",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_apiextensions_apiserver",
    commit = "0fe22c71c47604641d9aa352c785b7912c200562",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/apiextensions-apiserver",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_apimachinery",
    commit = "2b1284ed4c93a43499e781493253e2ac5959c4fd",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/apimachinery",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_client_go",
    commit = "8d9ed539ba3134352c586810e749e58df4e94e4f",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/client-go",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_code_generator",
    commit = "3a2206dd6a78497deceb3ae058417fdeb2036c7e",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/code-generator",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_gengo",
    commit = "fd15ee9cc2f77baa4f31e59e6acbf21146455073",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/gengo",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_klog",
    commit = "a5bc97fbc634d635061f3146511332c7e313a55a",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/klog",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_kube_openapi",
    commit = "0317810137be915b9cf888946c6e115c1bfac693",
    build_file_proto_mode = "disable_global",
    importpath = "k8s.io/kube-openapi",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_sigs_controller_runtime",
    commit = "12d98582e72927b6cd0123e2b4e819f9341ce62c",
    build_file_proto_mode = "disable_global",
    importpath = "sigs.k8s.io/controller-runtime",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_sigs_controller_tools",
    commit = "fbf141159251d035089e7acdd5a343f8cec91b94",
    build_file_proto_mode = "disable_global",
    importpath = "sigs.k8s.io/controller-tools",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_sigs_testing_frameworks",
    commit = "d348cb12705b516376e0c323bacca72b00a78425",
    build_file_proto_mode = "disable_global",
    importpath = "sigs.k8s.io/testing_frameworks",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "io_k8s_sigs_yaml",
    commit = "fd68e9863619f6ec2fdd8625fe1f02e7c877e480",
    build_file_proto_mode = "disable_global",
    importpath = "sigs.k8s.io/yaml",
    build_extra_args = ["-exclude=vendor"],
)

go_repository(
    name = "org_golang_google_appengine",
    commit = "4a4468ece617fc8205e99368fa2200e9d1fad421",
    importpath = "google.golang.org/appengine",
)

go_repository(
    name = "org_golang_x_crypto",
    commit = "505ab145d0a99da450461ae2c1a9f6cd10d1f447",
    importpath = "golang.org/x/crypto",
)

go_repository(
    name = "org_golang_x_net",
    commit = "891ebc4b82d6e74f468c533b06f983c7be918a96",
    importpath = "golang.org/x/net",
)

go_repository(
    name = "org_golang_x_oauth2",
    commit = "d668ce993890a79bda886613ee587a69dd5da7a6",
    importpath = "golang.org/x/oauth2",
)

go_repository(
    name = "org_golang_x_sys",
    commit = "4d1cda033e0619309c606fc686de3adcf599539e",
    importpath = "golang.org/x/sys",
)

go_repository(
    name = "org_golang_x_text",
    commit = "f21a4dfb5e38f5895301dc265a8def02365cc3d0",
    importpath = "golang.org/x/text",
)

go_repository(
    name = "org_golang_x_time",
    commit = "85acf8d2951cb2a3bde7632f9ff273ef0379bcbd",
    importpath = "golang.org/x/time",
)

go_repository(
    name = "org_golang_x_tools",
    commit = "3c39ce7b61056afe4473b651789da5f89d4aeb20",
    importpath = "golang.org/x/tools",
)

go_repository(
    name = "org_uber_go_atomic",
    commit = "1ea20fb1cbb1cc08cbd0d913a96dead89aa18289",
    importpath = "go.uber.org/atomic",
)

go_repository(
    name = "org_uber_go_multierr",
    commit = "3c4937480c32f4c13a875a1829af76c98ca3d40a",
    importpath = "go.uber.org/multierr",
)

go_repository(
    name = "org_uber_go_zap",
    commit = "ff33455a0e382e8a81d14dd7c922020b6b5e7982",
    importpath = "go.uber.org/zap",
)
