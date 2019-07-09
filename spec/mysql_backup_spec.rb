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
      instances: [Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address')],
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
    context('when cluster_ips are provided') do
       let(:spec) {{
        "cf-mysql-backup" => {
          "cluster_ips" => [
            "127.0.0.1"
          ]

        }
       }}

       it 'use cluster_ips as Ips in the config' do
         tpl_output = template.render(spec, consumes: links)
         expect(tpl_output).to include('Ips: ["127.0.0.1"]')
       end
    end
  end
end
