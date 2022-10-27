require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'streaming-mysql-backup-tool job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('streaming-mysql-backup-tool') }

  describe 'streaming-mysql-backup-tool template' do
    let(:template) { job.template('config/streaming-mysql-backup-tool.yml') }

    context('when xtrabackup_path is provided') do
      let(:spec) {{
        "cf-mysql-backup" => {
          'xtrabackup_path' => 'VALUE',
          'tls' => {
            'server_certificate' => 'some-cert',
            'server_key' => 'some-key',
            'server_name' => 'some-server-name'
          },
          'endpoint_credentials' => {
            'username' => 'some-username',
            'password' => 'some-password'
          }
        }
      }}

      it 'renders the xtrabackup path' do
        tpl_output = template.render(spec)
        tpl_yaml = YAML.load(tpl_output)
        expect(tpl_yaml['Command']).to eq('VALUE/xtrabackup --defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf --backup --stream=tar --target-dir=/var/vcap/store/xtrabackup_tmp/')
      end
    end

    context('when mutual tls is not set') do
      context('when basic-auth is not provided') do
        let(:spec) {{
          "cf-mysql-backup" => {
            'tls' => {
              'server_certificate' => 'some-cert',
              'server_key' => 'some-key',
              'server_name' => 'some-server-name'
            }
          }
        }}

        it 'raise an error' do
          expect{template.render(spec)}.to raise_error(/Can't find property.*/)
        end

      end

      context('when basic-auth is provided') do
        let(:spec) {{
          "cf-mysql-backup" => {
            'tls' => {
              'server_certificate' => 'some-cert',
              'server_key' => 'some-key',
              'server_name' => 'some-server-name'
            },
            'endpoint_credentials' => {
              'username' => 'some-username',
              'password' => 'some-password'
            }
          }
        }}

        it 'configure with basic-auth info' do
          tpl_output = template.render(spec)
          tpl_yaml = YAML.load(tpl_output)
          expect(tpl_yaml['TLS']['EnableMutualTLS']).to equal(false)
          expect(tpl_yaml['TLS']['ServerCert']).to eq('some-cert')
          expect(tpl_yaml['TLS']['ServerKey']).to eq('some-key')
          expect(tpl_yaml['TLS']['ClientCA']).to be_nil
          expect(tpl_yaml['TLS']['RequiredClientIdentities']).to be_nil
          expect(tpl_yaml['Credentials']['Username']).to eq('some-username')
          expect(tpl_yaml['Credentials']['Password']).to eq('some-password')
        end
      end
    end

    context('when mutual tls is set') do
      context('when basic-auth is not provided') do
        let(:spec) {{
          "cf-mysql-backup" => {
            'enable_mutual_tls' => true,
            'tls' => {
              'client_ca' => 'some-ca',
              'server_certificate' => 'some-cert',
              'server_key' => 'some-key',
              'client_hostnames' => ['hostname1', 'hostname2'],
              'server_name' => 'some-server-name'
            }
          }
        }}

        it 'configure mutual TLS' do
          tpl_output = template.render(spec)
          tpl_yaml = YAML.load(tpl_output)
          expect(tpl_yaml['TLS']['EnableMutualTLS']).to equal(true)
          expect(tpl_yaml['TLS']['ServerCert']).to eq('some-cert')
          expect(tpl_yaml['TLS']['ServerKey']).to eq('some-key')
          expect(tpl_yaml['TLS']['ClientCA']).to eq('some-ca')
          expect(tpl_yaml['TLS']['RequiredClientIdentities'].size).to eq(2)
          expect(tpl_yaml['TLS']['RequiredClientIdentities']).to include('hostname1')
          expect(tpl_yaml['TLS']['RequiredClientIdentities']).to include('hostname2')
          expect(tpl_yaml['Credentials']).to be_empty
        end
      end

      context('when basic-auth is provided') do
        let(:spec) {{
          "cf-mysql-backup" => {
            'enable_mutual_tls' => true,
            'tls' => {
              'client_ca' => 'some-ca',
              'server_certificate' => 'some-cert',
              'server_key' => 'some-key',
              'client_hostnames' => ['hostname1', 'hostname2'],
              'server_name' => 'some-server-name'
            },
            'endpoint_credentials' => {
              'username' => 'some-username',
              'password' => 'some-password'
            }
          }
        }}

        it 'configure mutual TLS ignoring username and password' do
          tpl_output = template.render(spec)
          tpl_yaml = YAML.load(tpl_output)
          expect(tpl_yaml['TLS']['EnableMutualTLS']).to equal(true)
          expect(tpl_yaml['TLS']['ServerCert']).to eq('some-cert')
          expect(tpl_yaml['TLS']['ServerKey']).to eq('some-key')
          expect(tpl_yaml['TLS']['ClientCA']).to eq('some-ca')
          expect(tpl_yaml['TLS']['RequiredClientIdentities'].size).to eq(2)
          expect(tpl_yaml['TLS']['RequiredClientIdentities']).to include('hostname1')
          expect(tpl_yaml['TLS']['RequiredClientIdentities']).to include('hostname2')
          expect(tpl_yaml['Credentials']).to be_empty
        end
      end

    end
  end
end
