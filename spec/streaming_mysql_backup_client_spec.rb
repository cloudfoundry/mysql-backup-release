require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'streaming-mysql-backup-client job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('streaming-mysql-backup-client') }
  let(:links) {[
    Bosh::Template::Test::Link.new(
      name: 'mysql-backup-tool',
      instances: [
        Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-1', id: 'instance-id-1'),
        Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-2', id: 'instance-id-2')
      ],
      properties: {
        'cf-mysql-backup' => {
          'endpoint_credentials' => {
            'username' => 'some-username',
            'password' => 'some-password'
          }
        }
      }
    )
  ]}

  describe 'streaming-mysql-backup-client template' do
    let(:template) { job.template('config/streaming-mysql-backup-client.yml') }
    context('when backup_local_node_only is true') do
      let(:spec) {{
        "cf-mysql-backup" => {
          "backup_local_node_only" => true,
          'symmetric_key' => 'some-symmetric-key',
          'tls' => {
            'ca_certificate' => 'some-ca'
          }
        }
      }}

      it 'use 127.0.0.1 as Instances in the config' do
        tpl_output = template.render(spec, consumes: links)
        tpl_yaml = YAML.load(tpl_output)
        expect(tpl_yaml['Instances'].size).to equal(1)
        expect(tpl_yaml['Instances']).to contain_exactly({ "Address" => "127.0.0.1", "UUID" => "xxxxxx-xxxxxxxx-xxxxx"})
      end
    end

    context('when backup_local_node_only is not set') do
      let(:spec) {{
        "cf-mysql-backup" => {
          'symmetric_key' => 'some-symmetric-key',
          'tls' => {
            'ca_certificate' => 'some-ca'
          }
        }
      }}

      it 'use backup tool links as Ips in the config' do
        tpl_output = template.render(spec, consumes: links)
        tpl_yaml = YAML.load(tpl_output)

        expect(tpl_yaml['Instances'].size).to equal(2)
        expect(tpl_yaml['Instances']).to contain_exactly(
          { "Address" => "backup-instance-address-1", "UUID" => "instance-id-1" },
          { "Address" => "backup-instance-address-2", "UUID" => "instance-id-2" },
        )
      end
    end

    context('when mutual tls is not set') do
      let(:spec) {{
        "cf-mysql-backup" => {
          'symmetric_key' => 'some-symmetric-key',
          'tls' => {
            'ca_certificate' => 'some-ca',
            'server_name' => 'some-server-name'
          }
        }
      }}

      it 'configure without mutual TLS' do
        tpl_output = template.render(spec, consumes: links)
        tpl_yaml = YAML.load(tpl_output)
        expect(tpl_yaml['TLS']['EnableMutualTLS']).to equal(false)
        expect(tpl_yaml['TLS']['ServerName']).to eq('some-server-name')
        expect(tpl_yaml['TLS']['ServerCACert']).to eq('some-ca')
        expect(tpl_yaml['Credentials']['Username']).to eq('some-username')
        expect(tpl_yaml['Credentials']['Password']).to eq('some-password')
      end

      context('when basic-auth are not provided') do
        let(:links) {[
          Bosh::Template::Test::Link.new(
            name: 'mysql-backup-tool',
            instances: [
              Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-1'),
              Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-2')
            ],
          )
        ]}

        it 'raise an error' do
          expect{template.render(spec, consumes: links)}.to raise_error(/Can't find property.*/)
        end
      end
    end

    context('when mutual tls is set') do
      let(:spec) {{
        "cf-mysql-backup" => {
          'enable_mutual_tls' => true,
          'symmetric_key' => 'some-symmetric-key',
          'tls' => {
            'ca_certificate' => 'some-ca',
            'client_certificate' => 'some-cert',
            'client_key' => 'some-key',
            'server_name' => 'some-server-name'
          }
        }
      }}

      it 'configure mutual TLS while ignoring username and password' do
        tpl_output = template.render(spec, consumes: links)
        tpl_yaml = YAML.load(tpl_output)
        expect(tpl_yaml['TLS']['EnableMutualTLS']).to equal(true)
        expect(tpl_yaml['TLS']['ClientCert']).to eq('some-cert')
        expect(tpl_yaml['TLS']['ClientKey']).to eq('some-key')
        expect(tpl_yaml['TLS']['ServerName']).to eq('some-server-name')
        expect(tpl_yaml['TLS']['ServerCACert']).to eq('some-ca')
        expect(tpl_yaml['Credentials']).to be_empty
      end

      context 'link does not contain username and password' do
        let(:links) {[
          Bosh::Template::Test::Link.new(
            name: 'mysql-backup-tool',
            instances: [
              Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-1'),
              Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-2')
            ],
          )
        ]}

        it 'configure mutual TLS' do
          tpl_output = template.render(spec, consumes: links)
          tpl_yaml = YAML.load(tpl_output)
          expect(tpl_yaml['TLS']['EnableMutualTLS']).to equal(true)
          expect(tpl_yaml['TLS']['ClientCert']).to eq('some-cert')
          expect(tpl_yaml['TLS']['ClientKey']).to eq('some-key')
          expect(tpl_yaml['TLS']['ServerName']).to eq('some-server-name')
          expect(tpl_yaml['TLS']['ServerCACert']).to eq('some-ca')
          expect(tpl_yaml['Credentials']).to be_empty
        end
      end
    end
  end
end
